package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tommahs/trinity-cache/internal/metrics"
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
	cache := &MockCache{}
	server, err := NewHTTPServer(cache, ":9000")

	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	if server == nil {
		t.Errorf("server should not be nil")
	}

	if server.cacheManager != cache {
		t.Errorf("cache not set correctly")
	}
}

func TestHTTPServer_New_NilCache(t *testing.T) {
	_, err := NewHTTPServer(nil, ":9000")
	if err == nil {
		t.Errorf("expected error for nil cache")
	}
}

func TestHTTPServer_StartAndShutdown(t *testing.T) {
	cache := &MockCache{}
	server, _ := NewHTTPServer(cache, ":0") // Use port 0 for automatic assignment

	err := server.Start()
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
	cache := &MockCache{}
	server, _ := NewHTTPServer(cache, ":0")

	server.Start()
	defer server.Shutdown(context.Background())

	err := server.Start()
	if err == nil {
		t.Errorf("expected error on double start")
	}
}

func TestHTTPServer_HandleHealth(t *testing.T) {
	cache := &MockCache{}
	server, _ := NewHTTPServer(cache, ":0")

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

	cache := &MockCache{}
	server, _ := NewHTTPServer(cache, ":0")

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
	cache := &MockCache{}
	server, _ := NewHTTPServer(cache, ":0")

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
	cache := &MockCache{}
	server, _ := NewHTTPServer(cache, ":0")

	req := httptest.NewRequest("GET", "/api/v1/packages/myapp/1.0", nil)
	w := httptest.NewRecorder()

	server.handlePackageRequest(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHTTPServer_HandlePackageRequest_HEAD(t *testing.T) {
	cache := &MockCache{}
	server, _ := NewHTTPServer(cache, ":0")

	req := httptest.NewRequest("HEAD", "/api/v1/packages/myapp/1.0", nil)
	w := httptest.NewRecorder()

	server.handlePackageRequest(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHTTPServer_HandlePackageRequest_MethodNotAllowed(t *testing.T) {
	cache := &MockCache{}
	server, _ := NewHTTPServer(cache, ":0")

	req := httptest.NewRequest("POST", "/api/v1/packages/myapp/1.0", nil)
	w := httptest.NewRecorder()

	server.handlePackageRequest(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHTTPServer_SetCache(t *testing.T) {
	cache1 := &MockCache{}
	cache2 := &MockCache{}

	server, _ := NewHTTPServer(cache1, ":0")

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
	cache := &MockCache{}
	server, _ := NewHTTPServer(cache, ":0")

	err := server.FetchAndServe("app", "1.0")
	if err != nil {
		t.Errorf("FetchAndServe should not error: %v", err)
	}
}

func TestHTTPServer_GracefulShutdownWithTimeout(t *testing.T) {
	cache := &MockCache{}
	server, _ := NewHTTPServer(cache, ":0")

	server.Start()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := server.Shutdown(ctx)
	if err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}

func TestHTTPServer_HandleFetchRequest_NoManager(t *testing.T) {
	cache := &MockCache{}
	server, _ := NewHTTPServer(cache, ":0")

	req := httptest.NewRequest("POST", "/api/v1/fetch/myapp/1.0", nil)
	w := httptest.NewRecorder()

	server.handleFetchRequest(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHTTPServer_HandleFetchRequest_InvalidPath(t *testing.T) {
	cache := &MockCache{}
	server, _ := NewHTTPServer(cache, ":0")

	req := httptest.NewRequest("POST", "/api/v1/fetch/", nil)
	w := httptest.NewRecorder()

	server.handleFetchRequest(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHTTPServer_HandleFetchRequest_MethodNotAllowed(t *testing.T) {
	cache := &MockCache{}
	server, _ := NewHTTPServer(cache, ":0")

	req := httptest.NewRequest("DELETE", "/api/v1/fetch/myapp/1.0", nil)
	w := httptest.NewRecorder()

	server.handleFetchRequest(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHTTPServer_SetFetchManager(t *testing.T) {
	cache := &MockCache{}
	server, _ := NewHTTPServer(cache, ":0")

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
