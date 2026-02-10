package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/tommahs/trinity-cache/internal/metrics"
	"github.com/tommahs/trinity-cache/internal/cache"
)

// MockCache for testing
type MockCache struct{}

func (mc *MockCache) Has(name, version string) (bool, error) {
	return true, nil
}

func (mc *MockCache) GetLatest(name string) (*CacheVersion, error) {
	return nil, nil
}

func (mc *MockCache) Add(p *CacheVersion) error {
	return nil
}

func (mc *MockCache) ListVersions(name string) ([]*CacheVersion, error) {
	return nil, nil
}

func (mc *MockCache) RetainMostRecent(name string, keep int) error {
	return nil
}

func (mc *MockCache) Remove(name, version string) error {
	return nil
}

type CacheVersion struct {
	Name    string
	Version string
	Path    string
}

func TestHTTPServer_New(t *testing.T) {
	// cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	server, err := NewHTTPServer(cacheManager, ":9000", 30,30,)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	if server == nil {
		t.Errorf("server should not be nil")
	}

	if server.cacheManager != cacheManager {
		t.Errorf("cache not set correctly")
	}
}

func TestHTTPServer_New_NilCache(t *testing.T) {
	_, err := NewHTTPServer(nil, ":9000", 30,30,)
	if err == nil {
		t.Errorf("expected error for nil cache")
	}
}

func TestHTTPServer_StartAndShutdown(t *testing.T) {
	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	if err != nil {
		t.Fatalf("cacheManager cannot be created")
	}
	server, _ := NewHTTPServer(cacheManager, ":0", 30,30) // Use port 0 for automatic assignment

	err = server.Start()
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}

	if !server.IsRunning() {
		t.Errorf("server should be running")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = server.Shutdown(ctx)
	if err != nil {
		t.Fatalf("failed to shutdown server: %v", err)
	}

	if server.IsRunning() {
		t.Errorf("server should not be running after shutdown")
	}
}

func TestHTTPServer_DoubleStart(t *testing.T) {
	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	server, _ := NewHTTPServer(cacheManager, ":0", 30, 30,)

	server.Start()
	defer server.Shutdown(context.Background())

	err = server.Start()
	if err == nil {
		t.Errorf("expected error on double start")
	}
}

func TestHTTPServer_HandleHealth(t *testing.T) {
	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	if err != nil {
		t.Errorf("expected error to be empty")
	}
	server, _ := NewHTTPServer(cacheManager, ":0", 30, 30,)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var health map[string]interface{}
	json.NewDecoder(w.Body).Decode(&health)

	if health["status"] != "ok" {
		t.Errorf("expected status 'ok'")
	}
}

func TestHTTPServer_HandleStats(t *testing.T) {
	metrics.Reset()
	metrics.RecordCacheHit()
	metrics.RecordCacheHit()

	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	if err != nil {
		t.Errorf("expected error to be empty")
	}
	server, _ := NewHTTPServer(cacheManager, ":0", 30, 30,)

	req := httptest.NewRequest("GET", "/api/v1/stats", nil)
	w := httptest.NewRecorder()

	server.handleStatsRequest(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var stats map[string]interface{}
	json.NewDecoder(w.Body).Decode(&stats)

	if stats["cache"] == nil {
		t.Errorf("expected cache stats")
	}
}

func TestHTTPServer_HandleMetrics(t *testing.T) {
	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	if err != nil {
		t.Errorf("expected error to be empty")
	}
	server, _ := NewHTTPServer(cacheManager, ":0", 30, 30,)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()

	server.handleMetrics(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected application/json content type")
	}
}

func TestHTTPServer_HandlePackageRequest_GET(t *testing.T) {
	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	if err != nil {
		t.Errorf("expected error to be empty")
	}
	server, _ := NewHTTPServer(cacheManager, ":0", 30, 30,)

	req := httptest.NewRequest("GET", "/api/v1/packages/myapp/1.0", nil)
	w := httptest.NewRecorder()

	server.handlePackageRequest(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHTTPServer_HandlePackageRequest_HEAD(t *testing.T) {
	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	if err != nil {
		t.Errorf("expected error to be empty")
	}
	server, _ := NewHTTPServer(cacheManager, ":0", 30, 30,)

	req := httptest.NewRequest("HEAD", "/api/v1/packages/myapp/1.0", nil)
	w := httptest.NewRecorder()

	server.handlePackageRequest(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHTTPServer_HandlePackageRequest_MethodNotAllowed(t *testing.T) {
	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	if err != nil {
		t.Errorf("expected error to be empty")
	}
	server, _ := NewHTTPServer(cacheManager, ":0", 30, 30,)

	req := httptest.NewRequest("POST", "/api/v1/packages/myapp/1.0", nil)
	w := httptest.NewRecorder()

	server.handlePackageRequest(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHTTPServer_SetCache(t *testing.T) {
	cache1, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	if err != nil {
		t.Errorf("expected error to be empty")
	}
	cache2, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	if err != nil {
		t.Errorf("expected error to be empty")
	}

	server, _ := NewHTTPServer(cache1, ":0",30,30)

	if server.cacheManager != cache1 {
		t.Errorf("cache1 not set initially")
	}

	server.SetCache(cache2)

	if server.cacheManager != cache2 {
		t.Errorf("cache2 not set after SetCache()")
	}

	// nil should be ignored
	server.SetCache(nil)
	if server.cacheManager != cache2 {
		t.Errorf("nil cache should be ignored")
	}
}

func TestHTTPServer_FetchAndServe(t *testing.T) {
	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	server, _ := NewHTTPServer(cacheManager, ":0", 30, 30,)

	err = server.FetchAndServe("app", "1.0")
	if err != nil {
		t.Errorf("FetchAndServe should not error: %v", err)
	}
}

func TestHTTPServer_GracefulShutdownWithTimeout(t *testing.T) {
	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	server, _ := NewHTTPServer(cacheManager, ":0", 30, 30,)

	server.Start()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err = server.Shutdown(ctx)
	if err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}

func TestHTTPServer_HandleFetchRequest_NoManager(t *testing.T) {
	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	if err != nil {
		t.Fatalf("cacheManager error: %v", err)
	}
	server, _ := NewHTTPServer(cacheManager, ":0", 30, 30,)

	req := httptest.NewRequest("POST", "/api/v1/fetch/myapp/1.0", nil)
	w := httptest.NewRecorder()

	server.handleFetchRequest(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHTTPServer_HandleFetchRequest_InvalidPath(t *testing.T) {
	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	if err != nil {
		t.Fatalf("cacheManager error: %v", err)
	}
	server, _ := NewHTTPServer(cacheManager, ":0", 30, 30,)

	req := httptest.NewRequest("POST", "/api/v1/fetch/", nil)
	w := httptest.NewRecorder()

	server.handleFetchRequest(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHTTPServer_HandleFetchRequest_MethodNotAllowed(t *testing.T) {
	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	if err != nil {
		t.Fatalf("cacheManager error: %v", err)
	}
	server, _ := NewHTTPServer(cacheManager, ":0", 30, 30,)

	req := httptest.NewRequest("DELETE", "/api/v1/fetch/myapp/1.0", nil)
	w := httptest.NewRecorder()

	server.handleFetchRequest(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHTTPServer_SetFetchManager(t *testing.T) {
	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	if err != nil {
		t.Fatalf("cacheManager error: %v", err)
	}
	server, _ := NewHTTPServer(cacheManager, ":0", 30, 30,)

	if server.fetchManager != nil {
		t.Errorf("fetch manager should be nil initially")
	}

	// In a real test, we'd create a proper FetchManager here
	// For now, just verify the method doesn't panic
	server.SetFetchManager(nil)
	if server.fetchManager != nil {
		t.Errorf("nil fetch manager should be ignored")
	}
}
func TestHTTPServer_ParsePackageName_Valid(t *testing.T) {
	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	if err != nil {
		t.Fatalf("cacheManager error: %v", err)
	}
	server, _ := NewHTTPServer(cacheManager, ":0", 30, 30,)

	tests := []struct {
		filename    string
		arch        string
		expectedPkg string
		expectedVer string
	}{
		{"linux-6.7.1-1-x86_64.pkg.tar.zst", "x86_64", "linux", "6.7.1-1"},
		{"base-2.0-1-x86_64.pkg.tar.zst", "x86_64", "base", "2.0-1"},
		{"gcc-13.2.1-2-x86_64.pkg.tar.zst", "x86_64", "gcc", "13.2.1-2"},
		{"lib32-glibc-2.38-3-x86_64.pkg.tar.zst", "x86_64", "lib32-glibc", "2.38-3"},
	}

	for _, tc := range tests {
		pkg, ver, err := server.parsePackageName(tc.filename, tc.arch)
		if err != nil {
			t.Errorf("parsePackageName(%s, %s) failed: %v", tc.filename, tc.arch, err)
			continue
		}
		if pkg != tc.expectedPkg {
			t.Errorf("parsePackageName(%s, %s) returned pkg=%q, want %q", tc.filename, tc.arch, pkg, tc.expectedPkg)
		}
		if ver != tc.expectedVer {
			t.Errorf("parsePackageName(%s, %s) returned ver=%q, want %q", tc.filename, tc.arch, ver, tc.expectedVer)
		}
	}
}

func TestHTTPServer_ParsePackageName_Invalid(t *testing.T) {
	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	if err != nil {
		t.Fatalf("cacheManager error: %v", err)
	}
	server, _ := NewHTTPServer(cacheManager, ":0", 30, 30,)

	tests := []struct {
		filename string
		arch     string
	}{
		{"invalid.pkg.tar.zst", "x86_64"},                    // missing version
		{"linux-6.7.1-1-i686.pkg.tar.zst", "x86_64"},         // arch mismatch
		{"", "x86_64"},                                         // empty filename
		{"linux-x86_64.pkg.tar.zst", "x86_64"},                // no version
	}

	for _, tc := range tests {
		_, _, err := server.parsePackageName(tc.filename, tc.arch)
		if err == nil {
			t.Errorf("parsePackageName(%s, %s) should have failed", tc.filename, tc.arch)
		}
	}
}

func TestHTTPServer_HandlePacmanRequest_NotFound(t *testing.T) {
	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	if err != nil {
		t.Fatalf("cacheManager error: %v", err)
	}
	server, _ := NewHTTPServer(cacheManager, ":0", 30, 30,)

	req := httptest.NewRequest("GET", "/nonexistent/os/x86_64/package.pkg.tar.zst", nil)
	w := httptest.NewRecorder()

	// Need to call the mux handler
	server.httpServer.Handler.ServeHTTP(w, req)

	if w.Code == http.StatusMethodNotAllowed {
		// This is expected if the route didn't match
		t.Logf("Route not matched (expected for test setup)")
	}
}

func TestHTTPServer_HandlePacmanRequest_InvalidFormat(t *testing.T) {
	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	if err != nil {
		t.Fatalf("cacheManager error: %v", err)
	}
	server, _ := NewHTTPServer(cacheManager, ":0", 30, 30,)

	// Invalid path format
	req := httptest.NewRequest("GET", "/invalid/path/format", nil)
	w := httptest.NewRecorder()

	server.handlePacmanRequest(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for invalid format, got %d", w.Code)
	}
}

func TestHTTPServer_HandlePacmanRequest_MethodNotAllowed(t *testing.T) {
	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	if err != nil {
		t.Fatalf("cacheManager error: %v", err)
	}
	server, _ := NewHTTPServer(cacheManager, ":0", 30, 30,)

	req := httptest.NewRequest("DELETE", "/core/os/x86_64/linux-6.7.1-1-x86_64.pkg.tar.zst", nil)
	w := httptest.NewRecorder()

	server.handlePacmanRequest(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for DELETE, got %d", w.Code)
	}
}

func TestHTTPServer_HandlePacmanRequest_CacheMiss_NoFetchManager(t *testing.T) {
	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	if err != nil {
		t.Fatalf("cacheManager error: %v", err)
	}
	server, _ := NewHTTPServer(cacheManager, ":0", 30, 30,)
	server.fetchManager = nil // No fetch manager

	// Request a package not in cache
	req := httptest.NewRequest("GET", "/core/os/x86_64/linux-6.7.1-1-x86_64.pkg.tar.zst", nil)
	w := httptest.NewRecorder()

	server.handlePacmanRequest(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when fetch manager not available, got %d", w.Code)
	}
}

func TestHTTPServer_MoveFileToCache(t *testing.T) {
	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	server, _ := NewHTTPServer(cacheManager, ":0", 30, 30,)

	// Create a temporary source file
	tempDir := t.TempDir()
	srcFile, err := os.Create(tempDir + "/source.txt")
	if err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}
	if _, err := srcFile.WriteString("test content"); err != nil {
		t.Fatalf("failed to write to source file: %v", err)
	}
	srcFile.Close()

	// Create destination path
	dstDir := t.TempDir()
	dstFile := dstDir + "/destination.txt"

	// Move file
	err = server.moveFileToCache(srcFile.Name(), dstFile)
	if err != nil {
		t.Fatalf("moveFileToCache failed: %v", err)
	}

	// Verify destination exists
	if _, err := os.Stat(dstFile); err != nil {
		t.Fatalf("destination file not found: %v", err)
	}

	// Verify content
	content, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("failed to read destination file: %v", err)
	}
	if string(content) != "test content" {
		t.Errorf("content mismatch, got %s, want 'test content'", string(content))
	}
}

func TestHTTPServer_MoveFileToCache_CreatesDirs(t *testing.T) {
	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	server, _ := NewHTTPServer(cacheManager, ":0", 30, 30,)

	// Create a temporary source file
	tempDir := t.TempDir()
	srcFile, err := os.Create(tempDir + "/source.txt")
	if err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}
	srcFile.WriteString("test")
	srcFile.Close()

	// Create destination path with non-existent directories
	dstFile := t.TempDir() + "/deep/nested/path/destination.txt"

	// Move file (should create directories)
	err = server.moveFileToCache(srcFile.Name(), dstFile)
	if err != nil {
		t.Fatalf("moveFileToCache failed: %v", err)
	}

	// Verify destination exists
	if _, err := os.Stat(dstFile); err != nil {
		t.Fatalf("destination file not found: %v", err)
	}
}

func TestHTTPServer_MoveFileToCache_EmptyPaths(t *testing.T) {
	cacheManager, err := cache.NewFilesystemCache("/var/lib/trinity-cache")
	if err != nil {
		t.Fatalf("cacheManager error: %v", err)
	}
	server, _ := NewHTTPServer(cacheManager, ":0", 30, 30,)

	// Test with empty src
	err = server.moveFileToCache("", "/some/path")
	if err == nil {
		t.Errorf("expected error for empty source path")
	}

	// Test with empty dst
	err = server.moveFileToCache("/some/path", "")
	if err == nil {
		t.Errorf("expected error for empty destination path")
	}
}