package orchestrator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const steelStartupTimeout = 45 * time.Second

type steelSessionDetails struct {
	ID                string `json:"id"`
	Status            string `json:"status"`
	BrowserWSEndpoint string `json:"browserWSEndpoint"`
}

func createSteelSession(baseURL, profilePath string, headless bool, proxyURL string, extensionPaths []string) (*steelSessionDetails, error) {
	body := map[string]any{
		"persist":     true,
		"userDataDir": filepath.Clean(profilePath),
		"headless":    headless,
	}
	if strings.TrimSpace(proxyURL) != "" {
		body["proxyUrl"] = strings.TrimSpace(proxyURL)
	}
	if len(extensionPaths) > 0 {
		body["extra"] = map[string]any{
			"orgExtensions": map[string]any{
				"paths": extensionPaths,
			},
		}
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode steel session request: %w", err)
	}

	resp, err := (&http.Client{Timeout: 60 * time.Second}).Post(
		baseURL+"/v1/sessions",
		"application/json",
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("create steel session: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("create steel session returned %d", resp.StatusCode)
	}

	var details steelSessionDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, fmt.Errorf("decode steel session response: %w", err)
	}
	if details.BrowserWSEndpoint == "" {
		return nil, fmt.Errorf("steel session did not return browserWSEndpoint")
	}
	return &details, nil
}

func startSteelProcess(logBuf io.Writer, steelPort, cdpPort int) (*exec.Cmd, string, error) {
	repoDir, err := resolveSteelRepoDir()
	if err != nil {
		return nil, "", err
	}
	cwd, binary, args, err := steelLaunchCommand(repoDir)
	if err != nil {
		return nil, "", err
	}

	cmd := exec.Command(binary, args...)
	cmd.Dir = cwd
	cmd.Stdout = logBuf
	cmd.Stderr = logBuf
	cmd.Env = append(os.Environ(),
		"HOST=127.0.0.1",
		fmt.Sprintf("PORT=%d", steelPort),
		fmt.Sprintf("CHROME_DEBUG_PORT=%d", cdpPort),
		fmt.Sprintf("CDP_REDIRECT_PORT=%d", cdpPort),
		"AUTO_LAUNCH_BROWSER=false",
		"NODE_ENV=development",
	)
	if err := cmd.Start(); err != nil {
		return nil, "", fmt.Errorf("failed to start steel server: %w", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", steelPort)
	if err := waitForSteelHealth(baseURL); err != nil {
		stopExternalProcess(cmd)
		return nil, "", err
	}
	return cmd, baseURL, nil
}

func waitForSteelHealth(baseURL string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(steelStartupTimeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL + "/v1/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("steel health check timed out after %s", steelStartupTimeout)
}

func resolveSteelRepoDir() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("PINCHTAB_STEEL_DIR")); explicit != "" {
		if looksLikeSteelRepo(explicit) {
			return explicit, nil
		}
		return "", fmt.Errorf("PINCHTAB_STEEL_DIR does not point to a steel repo: %s", explicit)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	candidate := filepath.Join(home, "Desktop", "steel-browser")
	if looksLikeSteelRepo(candidate) {
		return candidate, nil
	}
	return "", fmt.Errorf("could not locate steel-browser repo; set PINCHTAB_STEEL_DIR")
}

func looksLikeSteelRepo(path string) bool {
	if path == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(path, "api", "package.json")); err != nil {
		return false
	}
	return true
}

func steelLaunchCommand(repoDir string) (cwd, binary string, args []string, err error) {
	buildEntry := filepath.Join(repoDir, "api", "build", "index.js")
	if _, statErr := os.Stat(buildEntry); statErr == nil {
		nodePath, lookErr := exec.LookPath("node")
		if lookErr != nil {
			return "", "", nil, fmt.Errorf("node executable not found in PATH: %w", lookErr)
		}
		return filepath.Join(repoDir, "api"), nodePath, []string{"./build/index.js"}, nil
	}
	tsxBin := filepath.Join(repoDir, "node_modules", ".bin", "tsx")
	if _, statErr := os.Stat(tsxBin); statErr == nil {
		return repoDir, tsxBin, []string{"api/src/index.ts"}, nil
	}
	return "", "", nil, fmt.Errorf("steel build or tsx runner not found in %s; run npm install first", repoDir)
}

func releaseSteelSession(baseURL, sessionID string) {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(baseURL) == "" {
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/v1/sessions/"+sessionID+"/release", nil)
	if err != nil {
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

func stopExternalProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	case <-done:
	}
}
