package cloudprofiles

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

func TestArchiveDirectoryIncludesOnlyPortableState(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(rel, value string) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", rel, err)
		}
	}

	mustWrite("Default/Cookies", "cookies")
	mustWrite("Default/Local Storage/leveldb/000003.log", "localstorage")
	mustWrite("Default/Cache/Cache_Data/data_0", "cache")
	mustWrite("Default/GPUCache/data_1", "gpu")
	mustWrite("profile.json", "{}")

	archivePath, _, _, err := archiveDirectory(root)
	if err != nil {
		t.Fatalf("archiveDirectory failed: %v", err)
	}
	defer func() { _ = os.Remove(archivePath) }()

	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("Open archive failed: %v", err)
	}
	defer func() { _ = f.Close() }()
	gzr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}
	defer func() { _ = gzr.Close() }()

	var names []string
	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tr.Next failed: %v", err)
		}
		names = append(names, hdr.Name)
	}

	got := strings.Join(names, "\n")
	for _, want := range []string{
		"Default/Cookies",
		"Default/Local Storage",
		"Default/Local Storage/leveldb",
		"Default/Local Storage/leveldb/000003.log",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("archive missing %q in:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{
		"Default/Cache/Cache_Data/data_0",
		"Default/GPUCache/data_1",
		"profile.json",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("archive unexpectedly included %q in:\n%s", unwanted, got)
		}
	}
}

func TestExtractArchiveClearsOnlyPortableState(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(rel, value string) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", rel, err)
		}
	}
	mustWrite("Default/Cookies", "newcookies")
	mustWrite("Default/Bookmarks", "newbookmarks")

	archivePath, _, _, err := archiveDirectory(root)
	if err != nil {
		t.Fatalf("archiveDirectory failed: %v", err)
	}
	defer func() { _ = os.Remove(archivePath) }()

	dest := t.TempDir()
	mustDestWrite := func(rel, value string) {
		path := filepath.Join(dest, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("dest MkdirAll(%q): %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
			t.Fatalf("dest WriteFile(%q): %v", rel, err)
		}
	}
	mustDestWrite("Default/Cookies", "oldcookies")
	mustDestWrite("Default/Cache/Cache_Data/data_0", "cache")

	if err := extractArchive(archivePath, dest); err != nil {
		t.Fatalf("extractArchive failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dest, "Default", "Cookies"))
	if err != nil {
		t.Fatalf("ReadFile cookies failed: %v", err)
	}
	if string(data) != "newcookies" {
		t.Fatalf("cookies = %q, want newcookies", string(data))
	}
	cacheData, err := os.ReadFile(filepath.Join(dest, "Default", "Cache", "Cache_Data", "data_0"))
	if err != nil {
		t.Fatalf("ReadFile cache failed: %v", err)
	}
	if string(cacheData) != "cache" {
		t.Fatalf("cache = %q, want preserved cache", string(cacheData))
	}
}

func TestValidateFrozenIdentityRequiresPinnedCloakFields(t *testing.T) {
	err := validateFrozenIdentity(&bridge.ProfileBackendPinchTab{
		ProxyURL:       "socks5://user:pass@1.2.3.4:1234",
		Timezone:       "America/Denver",
		Locale:         "en-US",
		Binary:         "/Users/test/.cloakbrowser/chromium-145.0.7632.109.2/Chromium.app/Contents/MacOS/Chromium",
		BrowserVersion: "145.0.7632.109",
		LaunchArgs: []string{
			"--fingerprint=42069",
			"--fingerprint-platform=windows",
			"--fingerprint-storage-quota=10000",
			"--fingerprint-timezone=America/Denver",
		},
	})
	if err != nil {
		t.Fatalf("validateFrozenIdentity returned error: %v", err)
	}

	err = validateFrozenIdentity(&bridge.ProfileBackendPinchTab{
		ProxyURL:       "socks5://user:pass@1.2.3.4:1234",
		Timezone:       "America/Denver",
		Locale:         "en-US",
		Binary:         "/Users/test/.cloakbrowser/chromium-145.0.7632.109.2/Chromium.app/Contents/MacOS/Chromium",
		BrowserVersion: "145.0.7632.109",
		LaunchArgs: []string{
			"--fingerprint=42069",
			"--fingerprint-storage-quota=10000",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "--fingerprint-platform") {
		t.Fatalf("expected fingerprint-platform validation error, got %v", err)
	}
}
