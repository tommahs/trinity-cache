package downloader

import (
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/tommahs/trinity-cache/internal/mirror"
)

// MockCache for testing
type MockCache struct {
	stored map[string]bool
}

func (mc *MockCache) GetPackagePath(name, version string) string {
	return fmt.Sprintf("/cache/%s/%s-%s.pkg", name, name, version)
}

func (mc *MockCache) Has(name, version string) (bool, error) {
	key := fmt.Sprintf("%s:%s", name, version)
	return mc.stored[key], nil
}

func (mc *MockCache) Add(name, version, path string) error {
	key := fmt.Sprintf("%s:%s", name, version)
	mc.stored[key] = true
	return nil
}

func (mc *MockCache) RetainMostRecent(name string, keep int) error {
	return nil
}

func TestHTTPDownloader_New(t *testing.T) {
	selector := mirror.NewWeightedSelector()
	cache := &MockCache{stored: make(map[string]bool)}
	tmpDir := t.TempDir()

	downloader, err := NewHTTPDownloader(selector, cache, tmpDir)
	if err != nil {
		t.Fatalf("failed to create downloader: %v", err)
	}

	if downloader == nil {
		t.Errorf("downloader should not be nil")
	}
}

func TestHTTPDownloader_New_NilSelector(t *testing.T) {
	cache := &MockCache{stored: make(map[string]bool)}
	tmpDir := t.TempDir()

	_, err := NewHTTPDownloader(nil, cache, tmpDir)
	if err == nil {
		t.Errorf("expected error for nil selector")
	}
}

func TestHTTPDownloader_Download_Success(t *testing.T) {
	// Create a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("package content"))
	}))
	defer server.Close()

	// Setup
	selector := mirror.NewWeightedSelector()
	selector.Add(&mirror.Mirror{
		URL:             server.URL,
		BaseWeight:      1.0,
		EffectiveWeight: 1.0,
	})

	cache := &MockCache{stored: make(map[string]bool)}
	tmpDir := t.TempDir()

	downloader, _ := NewHTTPDownloader(selector, cache, tmpDir)

	// Download
	result, err := downloader.Download(selector.List()[0], "test.pkg")
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	if result == nil {
		t.Errorf("result should not be nil")
	}

	if result.Size != 15 {
		t.Errorf("expected size 15, got %d", result.Size)
	}

	// Cleanup
	os.Remove(result.Path)
}

func TestHTTPDownloader_Download_NotFound(t *testing.T) {
	// Create a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	selector := mirror.NewWeightedSelector()
	selector.Add(&mirror.Mirror{
		URL:             server.URL,
		BaseWeight:      1.0,
		EffectiveWeight: 1.0,
	})

	cache := &MockCache{stored: make(map[string]bool)}
	tmpDir := t.TempDir()

	downloader, _ := NewHTTPDownloader(selector, cache, tmpDir)

	_, err := downloader.Download(selector.List()[0], "missing.pkg")
	if err == nil {
		t.Errorf("expected error for 404 response")
	}
}

func TestHTTPDownloader_SetRetries(t *testing.T) {
	selector := mirror.NewWeightedSelector()
	cache := &MockCache{stored: make(map[string]bool)}
	tmpDir := t.TempDir()

	downloader, _ := NewHTTPDownloader(selector, cache, tmpDir)

	downloader.SetRetries(5)
	if downloader.retries != 5 {
		t.Errorf("retries not updated")
	}

	// Invalid value should be ignored
	downloader.SetRetries(-1)
	if downloader.retries != 5 {
		t.Errorf("retries should not change for invalid value")
	}
}

func TestHTTPDownloader_Verify_Success(t *testing.T) {
	selector := mirror.NewWeightedSelector()
	cache := &MockCache{stored: make(map[string]bool)}
	tmpDir := t.TempDir()

	downloader, _ := NewHTTPDownloader(selector, cache, tmpDir)

	// Create a test file
	testFile := tmpDir + "/test.pkg"
	content := "test package content"
	ioutil.WriteFile(testFile, []byte(content), 0644)

	// Calculate expected checksum
	hash := sha256.Sum256([]byte(content))
	expectedChecksum := fmt.Sprintf("%x", hash)

	// Verify
	err := downloader.Verify(testFile, expectedChecksum)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
}

func TestHTTPDownloader_Verify_Mismatch(t *testing.T) {
	selector := mirror.NewWeightedSelector()
	cache := &MockCache{stored: make(map[string]bool)}
	tmpDir := t.TempDir()

	downloader, _ := NewHTTPDownloader(selector, cache, tmpDir)

	// Create a test file
	testFile := tmpDir + "/test.pkg"
	ioutil.WriteFile(testFile, []byte("content"), 0644)

	// Try to verify with wrong checksum
	err := downloader.Verify(testFile, "wrongchecksum")
	if err == nil {
		t.Errorf("expected error for mismatched checksum")
	}
}

func TestHTTPDownloader_GetPackageStatus(t *testing.T) {
	selector := mirror.NewWeightedSelector()
	cache := &MockCache{stored: make(map[string]bool)}
	cache.stored["app:1.0"] = true

	tmpDir := t.TempDir()
	downloader, _ := NewHTTPDownloader(selector, cache, tmpDir)

	exists, path, err := downloader.GetPackageStatus("app", "1.0")
	if err != nil {
		t.Fatalf("GetPackageStatus failed: %v", err)
	}

	if !exists {
		t.Errorf("package should exist")
	}

	if path == "" {
		t.Errorf("path should not be empty")
	}

	// Non-existent package
	exists, path, err = downloader.GetPackageStatus("app", "2.0")
	if err != nil {
		t.Fatalf("GetPackageStatus failed: %v", err)
	}

	if exists {
		t.Errorf("package should not exist")
	}
}
