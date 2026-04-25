package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/cloak"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

type uploadRequest struct {
	Selector string   `json:"selector"`
	Files    []string `json:"files"`
	Paths    []string `json:"paths"`
}

const (
	uploadSandboxDirName = "uploads"
	uploadStagingDirName = "upload-staging"
)

// HandleUpload sets files on an <input type="file"> element via CDP.
//
// POST /upload?tabId=<id>
//
//	{
//	  "selector": "input[type=file]",   // unified selector: CSS, XPath, text, ref, or semantic
//	  "files": ["data:image/png;base64,...", "base64:..."],
//	  "paths": ["uploads/photo.jpg"]
//	}
//
// Alternatively, callers can send multipart/form-data with one or more "file"
// parts plus an optional "selector" field. Uploaded files are staged in a temp
// dir and passed to CDP. Path-based uploads are limited to StateDir/uploads/.
func (h *Handlers) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if !h.Config.AllowUpload {
		httpx.ErrorCode(w, 403, "upload_disabled", httpx.DisabledEndpointMessage("upload", "security.allowUpload"), false, map[string]any{
			"setting": "security.allowUpload",
		})
		return
	}
	tabID := r.URL.Query().Get("tabId")
	maxRequestBytes := h.Config.EffectiveUploadMaxRequestBytes()
	maxFiles := h.Config.EffectiveUploadMaxFiles()
	maxFileBytes := h.Config.EffectiveUploadMaxFileBytes()
	maxTotalBytes := h.Config.EffectiveUploadMaxTotalBytes()

	r.Body = http.MaxBytesReader(w, r.Body, int64(maxRequestBytes))

	ctx, resolvedTabID, err := h.tabContext(r, tabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}
	owner := resolveOwner(r, "")
	if err := h.enforceTabLease(resolvedTabID, owner); err != nil {
		httpx.ErrorCode(w, http.StatusLocked, "tab_locked", err.Error(), false, nil)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}

	stagingDir, err := prepareUploadStagingDir(h.Config.StateDir, resolvedTabID)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("prepare upload staging: %w", err))
		return
	}

	req, tempFiles, cleanup, tempBytes, err := parseUploadRequest(r, stagingDir, maxFiles, maxFileBytes, maxTotalBytes)
	if err != nil {
		_ = os.RemoveAll(stagingDir)
		httpx.Error(w, 400, err)
		return
	}
	if cleanup != nil {
		defer func() {
			if cleanupErr := cleanup(); cleanupErr != nil {
				_ = os.RemoveAll(stagingDir)
			}
		}()
	}

	if req.Selector == "" {
		req.Selector = "input[type=file]"
	}
	uploadBase := filepath.Join(h.Config.StateDir, uploadSandboxDirName)
	if err := validateUploadPaths(req.Paths, uploadBase, maxFileBytes, maxTotalBytes, tempBytes); err != nil {
		_ = os.RemoveAll(stagingDir)
		httpx.Error(w, 400, err)
		return
	}

	allPaths := append(tempFiles, req.Paths...)
	if stagedPaths, err := h.stageCloakUploadPaths(resolvedTabID, allPaths); err != nil {
		_ = os.RemoveAll(stagingDir)
		httpx.Error(w, 500, fmt.Errorf("stage cloak upload: %w", err))
		return
	} else if len(stagedPaths) > 0 {
		allPaths = stagedPaths
	}

	tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer tCancel()
	go httpx.CancelOnClientDone(r.Context(), tCancel)

	// Find the file input node and set files via CDP.
	if err := chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Evaluate selector to get the DOM node.
			nodeID, err := resolveSelector(ctx, req.Selector)
			if err != nil {
				return fmt.Errorf("selector %q: %w", req.Selector, err)
			}
			return dom.SetFileInputFiles(allPaths).WithNodeID(nodeID).Do(ctx)
		}),
	); err != nil {
		_ = os.RemoveAll(stagingDir)
		httpx.Error(w, 500, fmt.Errorf("upload: %w", err))
		return
	}

	httpx.JSON(w, 200, map[string]any{
		"status": "ok",
		"files":  len(allPaths),
	})
}

func parseUploadRequest(r *http.Request, stagingDir string, maxFiles, maxFileBytes, maxTotalBytes int) (uploadRequest, []string, func() error, int64, error) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err == nil && mediaType == "multipart/form-data" {
		return parseMultipartUploadRequest(r, stagingDir, maxFiles, maxFileBytes, maxTotalBytes)
	}
	return parseJSONUploadRequest(r, stagingDir, maxFiles, maxFileBytes, maxTotalBytes)
}

func parseJSONUploadRequest(r *http.Request, stagingDir string, maxFiles, maxFileBytes, maxTotalBytes int) (uploadRequest, []string, func() error, int64, error) {
	var req uploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return uploadRequest{}, nil, nil, 0, fmt.Errorf("invalid JSON body: %w", err)
	}
	if len(req.Files) == 0 && len(req.Paths) == 0 {
		return uploadRequest{}, nil, nil, 0, fmt.Errorf("either 'files' (base64) or 'paths' (sandbox paths) required")
	}
	if len(req.Files)+len(req.Paths) > maxFiles {
		return uploadRequest{}, nil, nil, 0, fmt.Errorf("too many files: max %d", maxFiles)
	}

	if len(req.Files) == 0 {
		return req, nil, nil, 0, nil
	}

	var totalBytes int64
	tempFiles := make([]string, 0, len(req.Files))
	for i, f := range req.Files {
		data, ext, err := decodeFileData(f)
		if err != nil {
			return uploadRequest{}, nil, nil, 0, fmt.Errorf("file[%d]: %w", i, err)
		}
		if len(data) > maxFileBytes {
			return uploadRequest{}, nil, nil, 0, fmt.Errorf("file[%d] exceeds max size %d bytes", i, maxFileBytes)
		}
		totalBytes += int64(len(data))
		if totalBytes > int64(maxTotalBytes) {
			return uploadRequest{}, nil, nil, 0, fmt.Errorf("upload payload too large: max %d bytes total", maxTotalBytes)
		}
		path := filepath.Join(stagingDir, fmt.Sprintf("upload-%d%s", i, ext))
		if err := os.WriteFile(path, data, 0600); err != nil {
			return uploadRequest{}, nil, nil, 0, fmt.Errorf("write temp file: %w", err)
		}
		tempFiles = append(tempFiles, path)
	}

	return req, tempFiles, nil, totalBytes, nil
}

func parseMultipartUploadRequest(r *http.Request, stagingDir string, maxFiles, maxFileBytes, maxTotalBytes int) (uploadRequest, []string, func() error, int64, error) {
	reader, err := r.MultipartReader()
	if err != nil {
		return uploadRequest{}, nil, nil, 0, fmt.Errorf("invalid multipart body: %w", err)
	}

	req := uploadRequest{}
	var totalBytes int64
	fileIndex := 0
	tempFiles := make([]string, 0, 1)

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return uploadRequest{}, nil, nil, 0, fmt.Errorf("read multipart body: %w", err)
		}

		fieldName := part.FormName()
		filename := part.FileName()
		if filename == "" {
			value, readErr := io.ReadAll(io.LimitReader(part, 64<<10))
			_ = part.Close()
			if readErr != nil {
				return uploadRequest{}, nil, nil, 0, fmt.Errorf("read multipart field %q: %w", fieldName, readErr)
			}
			switch fieldName {
			case "selector":
				req.Selector = string(value)
			case "path", "paths":
				req.Paths = append(req.Paths, strings.TrimSpace(string(value)))
			}
			continue
		}

		if fieldName != "file" && fieldName != "files" {
			_ = part.Close()
			continue
		}
		if fileIndex+len(req.Paths) >= maxFiles {
			_ = part.Close()
			return uploadRequest{}, nil, nil, 0, fmt.Errorf("too many files: max %d", maxFiles)
		}

		tempPath, size, saveErr := saveMultipartFile(part, stagingDir, fileIndex, filename, maxFileBytes)
		_ = part.Close()
		if saveErr != nil {
			return uploadRequest{}, nil, nil, 0, saveErr
		}
		totalBytes += size
		if totalBytes > int64(maxTotalBytes) {
			return uploadRequest{}, nil, nil, 0, fmt.Errorf("upload payload too large: max %d bytes total", maxTotalBytes)
		}
		tempFiles = append(tempFiles, tempPath)
		fileIndex++
	}

	if len(tempFiles) == 0 && len(req.Paths) == 0 {
		return uploadRequest{}, nil, nil, 0, fmt.Errorf("either multipart 'file' parts or 'paths' fields required")
	}
	if len(tempFiles)+len(req.Paths) > maxFiles {
		return uploadRequest{}, nil, nil, 0, fmt.Errorf("too many files: max %d", maxFiles)
	}

	return req, tempFiles, nil, totalBytes, nil
}

func validateUploadPaths(paths []string, uploadBase string, maxFileBytes, maxTotalBytes int, baseBytes int64) error {
	totalBytes := baseBytes
	for i, p := range paths {
		safe, size, err := validateUploadSandboxPath(uploadBase, p, maxFileBytes)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		totalBytes += size
		if totalBytes > int64(maxTotalBytes) {
			return fmt.Errorf("upload payload too large: max %d bytes total", maxTotalBytes)
		}
		paths[i] = safe
	}
	return nil
}

func saveMultipartFile(part io.Reader, tmpDir string, index int, originalName string, maxFileBytes int) (string, int64, error) {
	header := make([]byte, 0, 512)
	limited := &io.LimitedReader{R: part, N: int64(maxFileBytes) + 1}
	chunk := make([]byte, 32<<10)

	for len(header) < cap(header) {
		need := cap(header) - len(header)
		if need > len(chunk) {
			need = len(chunk)
		}
		n, err := limited.Read(chunk[:need])
		if n > 0 {
			header = append(header, chunk[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", 0, fmt.Errorf("read multipart file: %w", err)
		}
		if n == 0 {
			break
		}
	}

	if len(header) > maxFileBytes {
		return "", 0, fmt.Errorf("multipart file[%d] exceeds max size %d bytes", index, maxFileBytes)
	}

	name := stagedUploadFilename(index, originalName, header)
	path := filepath.Join(tmpDir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return "", 0, fmt.Errorf("write temp file: %w", err)
	}
	defer func() { _ = f.Close() }()

	writtenHeader, err := f.Write(header)
	if err != nil {
		return "", 0, fmt.Errorf("write temp file: %w", err)
	}

	n, err := io.CopyBuffer(f, limited, chunk)
	size := int64(writtenHeader) + n
	if size > int64(maxFileBytes) {
		return "", 0, fmt.Errorf("multipart file[%d] exceeds max size %d bytes", index, maxFileBytes)
	}
	if err != nil {
		return "", 0, fmt.Errorf("write temp file: %w", err)
	}

	return path, size, nil
}

func stagedUploadFilename(index int, originalName string, header []byte) string {
	name := sanitizeUploadFilename(originalName)
	if name != "" {
		return name
	}
	return fmt.Sprintf("upload-%d%s", index, sniffExt(header))
}

func sanitizeUploadFilename(name string) string {
	base := filepath.Base(strings.TrimSpace(name))
	if base == "." || base == string(filepath.Separator) || base == "" {
		return ""
	}
	base = strings.Map(func(r rune) rune {
		switch {
		case r == 0:
			return -1
		case r == '/' || r == '\\':
			return -1
		case r < 32:
			return -1
		default:
			return r
		}
	}, base)
	base = strings.TrimSpace(base)
	base = strings.Trim(base, ".")
	if base == "" {
		return ""
	}
	return base
}

func prepareUploadStagingDir(stateDir, tabID string) (string, error) {
	root := uploadStagingRoot(stateDir)
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	stagingDir := filepath.Join(root, tabID)
	if err := os.RemoveAll(stagingDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(stagingDir, 0700); err != nil {
		return "", err
	}
	return stagingDir, nil
}

func cleanupUploadStagingDir(stateDir, tabID string) error {
	if tabID == "" {
		return nil
	}
	return os.RemoveAll(filepath.Join(uploadStagingRoot(stateDir), tabID))
}

func (h *Handlers) cleanupCloakUploadStagingDir(tabID string) error {
	if h == nil || h.Config == nil {
		return nil
	}
	if strings.TrimSpace(h.Config.CloakBaseURL) == "" || strings.TrimSpace(h.Config.CloakProfileID) == "" || strings.TrimSpace(tabID) == "" {
		return nil
	}
	client := cloak.NewClient(h.Config.CloakBaseURL)
	return client.CleanupUploadStage(h.Config.CloakProfileID, tabID)
}

func uploadStagingRoot(stateDir string) string {
	if strings.TrimSpace(stateDir) == "" {
		return filepath.Join(os.TempDir(), "pinchtab", uploadStagingDirName)
	}
	return filepath.Join(stateDir, uploadStagingDirName)
}

func (h *Handlers) stageCloakUploadPaths(tabID string, sourcePaths []string) ([]string, error) {
	if h == nil || h.Config == nil {
		return nil, nil
	}
	if strings.TrimSpace(h.Config.CloakBaseURL) == "" || strings.TrimSpace(h.Config.CloakProfileID) == "" {
		return nil, nil
	}
	client := cloak.NewClient(h.Config.CloakBaseURL)
	staged := make([]string, 0, len(sourcePaths))
	for _, sourcePath := range sourcePaths {
		resp, err := client.StageUpload(h.Config.CloakProfileID, tabID, sourcePath)
		if err != nil {
			return nil, err
		}
		staged = append(staged, resp.Path)
	}
	return staged, nil
}

// HandleTabUpload uploads files for a tab identified by path ID.
//
// @Endpoint POST /tabs/{id}/upload
func (h *Handlers) HandleTabUpload(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	q := r.URL.Query()
	q.Set("tabId", tabID)

	req := r.Clone(r.Context())
	u := *r.URL
	u.RawQuery = q.Encode()
	req.URL = &u

	h.HandleUpload(w, req)
}

// resolveSelector finds a DOM node by a unified selector string and returns its NodeID.
// Supports CSS (default), XPath (xpath: prefix or // auto-detect), and text (text: prefix).
func resolveSelector(ctx context.Context, sel string) (cdp.NodeID, error) {
	// Determine the JavaScript expression based on selector type.
	var expr string
	switch {
	case strings.HasPrefix(sel, "xpath:"):
		xpath := sel[len("xpath:"):]
		expr = fmt.Sprintf(`(function(){var r=document.evaluate(%q,document,null,XPathResult.FIRST_ORDERED_NODE_TYPE,null);return r.singleNodeValue})()`, xpath)
	case strings.HasPrefix(sel, "//") || strings.HasPrefix(sel, "(//"):
		expr = fmt.Sprintf(`(function(){var r=document.evaluate(%q,document,null,XPathResult.FIRST_ORDERED_NODE_TYPE,null);return r.singleNodeValue})()`, sel)
	case strings.HasPrefix(sel, "text:"):
		text := sel[len("text:"):]
		expr = fmt.Sprintf(`(function(){var w=document.createTreeWalker(document.body,NodeFilter.SHOW_TEXT);while(w.nextNode()){if(w.currentNode.textContent.includes(%q))return w.currentNode.parentElement}return null})()`, text)
	case strings.HasPrefix(sel, "css:"):
		css := sel[len("css:"):]
		expr = fmt.Sprintf(`document.querySelector(%q)`, css)
	default:
		// Bare selector — treat as CSS (backward compatible)
		expr = fmt.Sprintf(`document.querySelector(%q)`, sel)
	}

	val, _, err := runtime.Evaluate(expr).Do(ctx)
	if err != nil {
		return 0, fmt.Errorf("evaluate: %w", err)
	}
	if val.ObjectID == "" {
		return 0, fmt.Errorf("no element matches selector")
	}
	node, err := dom.RequestNode(val.ObjectID).Do(ctx)
	if err != nil {
		return 0, fmt.Errorf("request node: %w", err)
	}
	return node, nil
}

func validateUploadSandboxPath(baseDir, rawPath string, maxFileBytes int) (string, int64, error) {
	normalized := normalizeUploadSandboxPath(rawPath)
	safe, err := httpx.SafeExistingPath(baseDir, normalized)
	if err != nil {
		return "", 0, err
	}
	info, err := os.Lstat(safe)
	if err != nil {
		return "", 0, fmt.Errorf("file not found: %s", safe)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", 0, fmt.Errorf("symlinks are not allowed: %s", rawPath)
	}
	if !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("path must reference a regular file: %s", rawPath)
	}
	if info.Size() > int64(maxFileBytes) {
		return "", 0, fmt.Errorf("file exceeds max size %d bytes: %s", maxFileBytes, rawPath)
	}
	return safe, info.Size(), nil
}

func normalizeUploadSandboxPath(rawPath string) string {
	trimmed := filepath.ToSlash(strings.TrimSpace(rawPath))
	trimmed = strings.TrimPrefix(trimmed, uploadSandboxDirName+"/")
	return filepath.FromSlash(trimmed)
}

// decodeFileData handles "data:mime;base64,..." and raw base64 strings.
// Returns decoded bytes and a file extension guess.
func decodeFileData(input string) ([]byte, string, error) {
	ext := ""
	var b64 string

	if strings.HasPrefix(input, "data:") {
		// data:image/png;base64,iVBOR...
		parts := strings.SplitN(input, ",", 2)
		if len(parts) != 2 {
			return nil, "", fmt.Errorf("invalid data URL")
		}
		b64 = parts[1]
		// Extract mime for extension.
		meta := strings.TrimPrefix(parts[0], "data:")
		mime := strings.SplitN(meta, ";", 2)[0]
		ext = mimeToExt(mime)
	} else {
		b64 = input
	}

	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		// Try URL-safe encoding.
		data, err = base64.URLEncoding.DecodeString(b64)
		if err != nil {
			return nil, "", fmt.Errorf("base64 decode: %w", err)
		}
	}

	if ext == "" {
		ext = sniffExt(data)
	}

	return data, ext, nil
}

func mimeToExt(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/svg+xml":
		return ".svg"
	case "application/pdf":
		return ".pdf"
	case "text/plain":
		return ".txt"
	case "text/csv":
		return ".csv"
	default:
		return ".bin"
	}
}

func sniffExt(data []byte) string {
	if len(data) < 4 {
		return ".bin"
	}
	switch {
	case data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G':
		return ".png"
	case data[0] == 0xFF && data[1] == 0xD8:
		return ".jpg"
	case string(data[:3]) == "GIF":
		return ".gif"
	case string(data[:4]) == "RIFF" && len(data) > 11 && string(data[8:12]) == "WEBP":
		return ".webp"
	case data[0] == '%' && data[1] == 'P' && data[2] == 'D' && data[3] == 'F':
		return ".pdf"
	default:
		return ".bin"
	}
}
