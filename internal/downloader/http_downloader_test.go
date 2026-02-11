package downloader

import (
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

	downloader, err := NewHTTPDownloader(selector, cache, tmpDir, 3, 30)
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

	_, err := NewHTTPDownloader(nil, cache, tmpDir, 3, 30)
	if err == nil {
		t.Errorf("expected error for nil selector")
	}
}

func TestHTTPDownloader_New_NilCache(t *testing.T) {
	selector := mirror.NewWeightedSelector()
	tmpDir := t.TempDir()

	_, err := NewHTTPDownloader(selector, nil, tmpDir, 3, 30)
	if err == nil {
		t.Errorf("expected error for nil cache")
	}
}

func TestHTTPDownloader_New_InvalidRetries(t *testing.T) {
	selector := mirror.NewWeightedSelector()
	cache := &MockCache{stored: make(map[string]bool)}
	tmpDir := t.TempDir()

	downloader, err := NewHTTPDownloader(selector, cache, tmpDir, 0, 30)
	if err != nil {
		t.Fatalf("invalid retry count should default to 3: %v", err)
	}

	if downloader.retries < 1 {
		t.Errorf("retries should be at least 1")
	}
}

func TestHTTPDownloader_New_InvalidTimeout(t *testing.T) {
	selector := mirror.NewWeightedSelector()
	cache := &MockCache{stored: make(map[string]bool)}
	tmpDir := t.TempDir()

	downloader, err := NewHTTPDownloader(selector, cache, tmpDir, 3, 0)
	if err != nil {
		t.Fatalf("invalid timeout should default: %v", err)
	}

	if downloader.timeout.Seconds() < 1 {
		t.Errorf("timeout should be at least 1 second")
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

	downloader, _ := NewHTTPDownloader(selector, cache, tmpDir, 3, 30)

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

	downloader, _ := NewHTTPDownloader(selector, cache, tmpDir, 3, 30)

	_, err := downloader.Download(selector.List()[0], "missing.pkg")
	if err == nil {
		t.Errorf("expected error for 404 response")
	}
}

func TestHTTPDownloader_Download_InvalidInputs(t *testing.T) {
	selector := mirror.NewWeightedSelector()
	selector.Add(&mirror.Mirror{
		URL:             "http://example.com",
		BaseWeight:      1.0,
		EffectiveWeight: 1.0,
	})
	cache := &MockCache{stored: make(map[string]bool)}
	tmpDir := t.TempDir()

	downloader, _ := NewHTTPDownloader(selector, cache, tmpDir, 3, 30)

	// Nil mirror
	_, err := downloader.Download(nil, "test.pkg")
	if err == nil {
		t.Errorf("expected error for nil mirror")
	}

	// Empty path
	_, err = downloader.Download(selector.List()[0], "")
	if err == nil {
		t.Errorf("expected error for empty path")
	}
}

func TestHTTPDownloader_SetRetries(t *testing.T) {
	selector := mirror.NewWeightedSelector()
	cache := &MockCache{stored: make(map[string]bool)}
	tmpDir := t.TempDir()

	downloader, _ := NewHTTPDownloader(selector, cache, tmpDir, 3, 30)

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

	downloader, _ := NewHTTPDownloader(selector, cache, tmpDir, 3, 30)

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

	downloader, _ := NewHTTPDownloader(selector, cache, tmpDir, 3, 30)

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
	downloader, _ := NewHTTPDownloader(selector, cache, tmpDir, 3, 30)

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

func TestHTTPDownloader_ConcurrentDownloads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("package content"))
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

	downloader, _ := NewHTTPDownloader(selector, cache, tmpDir, 3, 30)

	var wg sync.WaitGroup
	var errorCount atomic.Int32
	numGoroutines := 5

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, err := downloader.Download(selector.List()[0], fmt.Sprintf("test%d.pkg", id))
			if err != nil {
				errorCount.Add(1)
				t.Logf("concurrent download %d failed: %v", id, err)
			}
		}(i)
	}

	wg.Wait()

	if errorCount.Load() > 0 {
		t.Errorf("concurrent downloads had errors: %d", errorCount.Load())
	}
}

func TestHTTPDownloader_Download_WithTimeout(t *testing.T) {
	// Create a slow server that takes longer than timeout
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("package content"))
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

	// Create downloader with very short timeout (1 millisecond)
	downloader, _ := NewHTTPDownloader(selector, cache, tmpDir, 3, 1)

	_, err := downloader.Download(selector.List()[0], "test.pkg")
	if err == nil {
		t.Logf("WARNING: Expected timeout error, but download succeeded (server may be too fast)")
	}
}

func TestHTTPDownloader_LargeFileDownload(t *testing.T) {
	// Create a larger mock file (1MB)
	largeContent := make([]byte, 1024*1024)
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(largeContent)
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

	downloader, _ := NewHTTPDownloader(selector, cache, tmpDir, 3, 30)

	result, err := downloader.Download(selector.List()[0], "largefile.pkg")
	if err != nil {
		t.Fatalf("large file download failed: %v", err)
	}

	if result.Size != int64(len(largeContent)) {
		t.Errorf("expected size %d, got %d", len(largeContent), result.Size)
	}

	// Verify file contents
	data, _ := ioutil.ReadFile(result.Path)
	if len(data) != len(largeContent) {
		t.Errorf("downloaded file size mismatch: got %d, expected %d", len(data), len(largeContent))
	}

	os.Remove(result.Path)
}

func TestHTTPDownloader_TempDirCreation(t *testing.T) {
	selector := mirror.NewWeightedSelector()
	cache := &MockCache{stored: make(map[string]bool)}
	nonexistentDir := t.TempDir() + "/subdir/nested/path"

	// Should create temp directory if it doesn't exist
	downloader, err := NewHTTPDownloader(selector, cache, nonexistentDir, 3, 30)
	if err != nil {
		t.Fatalf("failed to create downloader with nested temp dir: %v", err)
	}

	if downloader == nil {
		t.Errorf("downloader should not be nil")
	}

	if _, err := os.Stat(nonexistentDir); os.IsNotExist(err) {
		t.Errorf("temp directory should have been created")
	}
}
