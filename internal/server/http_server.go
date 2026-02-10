package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tommahs/trinity-cache/internal/cache"
	"github.com/tommahs/trinity-cache/internal/downloader"
	"github.com/tommahs/trinity-cache/internal/logger"
	"github.com/tommahs/trinity-cache/internal/metrics"
)

// HTTPServer implements the Server interface using HTTP
type HTTPServer struct {
	cacheManager    cache.CacheManager
	fetchManager    *downloader.FetchManager
	httpServer      *http.Server
	mu              sync.Mutex
	running         bool
	shutdownChan    chan struct{}
	allDone         chan struct{}
	activeRequests  int32
}

// NewHTTPServer creates a new HTTP server for package serving
func NewHTTPServer(cache cache.CacheManager, addr string) (*HTTPServer, error) {
	if cache == nil {
		return nil, fmt.Errorf("cache manager cannot be nil")
	}

	if addr == "" {
		addr = ":8080"
	}

	s := &HTTPServer{
		cacheManager: cache,
		shutdownChan: make(chan struct{}),
		allDone:      make(chan struct{}),
	}

	// Create HTTP server with routes
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/packages/", s.handlePackageRequest)
	mux.HandleFunc("/api/v1/fetch/", s.handleFetchRequest)
	mux.HandleFunc("/api/v1/stats", s.handleStatsRequest)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/metrics", s.handleMetrics)

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	return s, nil
}

// Start starts the HTTP server
func (s *HTTPServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("server already running")
	}

	s.running = true
	logger.Info("starting HTTP server", "addr", s.httpServer.Addr)

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrShutdown {
			logger.Error("HTTP server error", "error", err)
		}
	}()

	return nil
}

// Shutdown gracefully shuts down the server
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return fmt.Errorf("server not running")
	}
	s.mu.Unlock()

	logger.Info("shutting down HTTP server")

	// Signal shutdown
	close(s.shutdownChan)

	// Wait for graceful shutdown with timeout
	if ctx == nil {
		ctx = context.Background()
	}

	doneChan := make(chan error, 1)
	go func() {
		doneChan <- s.httpServer.Shutdown(ctx)
	}()

	select {
	case err := <-doneChan:
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		close(s.allDone)
		return err
	case <-ctx.Done():
		logger.Warn("HTTP server shutdown timeout, forcing close")
		s.httpServer.Close()
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		close(s.allDone)
		return ctx.Err()
	}
}

// IsRunning returns whether the server is running
func (s *HTTPServer) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// FetchAndServe implements the Server interface
func (s *HTTPServer) FetchAndServe(name, version string) error {
	// This would be implemented to ensure a package is available
	// For now, it's a placeholder
	logger.Debug("fetch and serve requested", "name", name, "version", version)
	return nil
}

// SetCache implements the Server interface
func (s *HTTPServer) SetCache(c cache.CacheManager) {
	if c != nil {
		s.cacheManager = c
	}
}

// SetFetchManager sets the fetch manager for on-demand fetching
func (s *HTTPServer) SetFetchManager(fm *downloader.FetchManager) {
	if fm != nil {
		s.fetchManager = fm
	}
}

// handlePackageRequest handles GET requests for packages
func (s *HTTPServer) handlePackageRequest(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt32(&s.activeRequests, 1)
	defer atomic.AddInt32(&s.activeRequests, -1)

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Parse package path from URL: /api/v1/packages/{name}/{version}
	path := r.URL.Path[len("/api/v1/packages/"):]
	if path == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error":"package path required"}`)
		return
	}

	logger.Debug("package request", "path", path)

	// For a real implementation, we'd parse name/version and serve the package
	metrics.RecordCacheHit()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
}

// handleFetchRequest handles on-demand fetch requests
func (s *HTTPServer) handleFetchRequest(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt32(&s.activeRequests, 1)
	defer atomic.AddInt32(&s.activeRequests, -1)

	if s.fetchManager == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"error":"fetch manager not available"}`)
		return
	}

	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Parse fetch request: /api/v1/fetch/{name}/{version}
	path := r.URL.Path[len("/api/v1/fetch/"):]
	if path == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error":"package name and version required"}`)
		return
	}

	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error":"invalid package path"}`)
		return
	}

	name := parts[0]
	version := parts[1]

	if name == "" || version == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error":"name and version required"}`)
		return
	}

	logger.Info("on-demand fetch requested", "name", name, "version", version)

	// For a real implementation, we'd construct the package path based on storage configuration
	pkgPath := fmt.Sprintf("/cache/%s/%s-%s.pkg", name, name, version)

	// Fetch the package
	result, err := s.fetchManager.FetchVersion(name, version, pkgPath)
	if err != nil {
		logger.Error("on-demand fetch failed", "name", name, "version", version, "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":"fetch failed: %s"}`, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"name":      name,
		"version":   version,
		"size":      result.Size,
		"sha256":    result.SHA256,
		"path":      result.Path,
		"duration":  result.Duration.Seconds(),
		"timestamp": time.Now(),
	})
}

// handleStatsRequest returns cache statistics
func (s *HTTPServer) handleStatsRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	snapshot := metrics.GetSnapshot()

	stats := map[string]interface{}{
		"timestamp": snapshot.Timestamp,
		"downloads": map[string]interface{}{
			"total":       snapshot.Downloads.Total,
			"successful":  snapshot.Downloads.Successful,
			"failed":      snapshot.Downloads.Failed,
			"bytes_total": snapshot.Downloads.BytesTotal,
			"avg_duration_ms": snapshot.Downloads.AvgDuration.Milliseconds(),
		},
		"cache": map[string]interface{}{
			"hits":     snapshot.Cache.Hits,
			"misses":   snapshot.Cache.Misses,
			"hit_rate": snapshot.Cache.HitRate,
			"packages": snapshot.Cache.Packages,
			"versions": snapshot.Cache.Versions,
		},
		"mirrors": map[string]interface{}{
			"selections": snapshot.Mirror.Selections,
			"penalties":  snapshot.Mirror.Penalties,
			"recoveries": snapshot.Mirror.Recoveries,
		},
		"retention": map[string]interface{}{
			"packages_removed": snapshot.Retention.PackagesRemoved,
			"versions_removed": snapshot.Retention.VersionsRemoved,
			"last_run":         snapshot.Retention.LastRunTime,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleHealth returns server health status
func (s *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	activeRequests := atomic.LoadInt32(&s.activeRequests)

	health := map[string]interface{}{
		"status":           "ok",
		"running":          s.IsRunning(),
		"active_requests":  activeRequests,
		"timestamp":        time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

// handleMetrics returns metrics in JSON format
func (s *HTTPServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	snapshot := metrics.GetSnapshot()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snapshot)
}

// ServePackage serves a package file directly
func (s *HTTPServer) ServePackage(w http.ResponseWriter, pkgPath string) error {
	// Open the package file
	file, err := os.Open(pkgPath)
	if err != nil {
		if os.IsNotExist(err) {
			w.WriteHeader(http.StatusNotFound)
			return err
		}
		w.WriteHeader(http.StatusInternalServerError)
		return err
	}
	defer file.Close()

	// Get file info for headers
	fi, err := file.Stat()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return err
	}

	// Set headers
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fi.Size()))
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	// Serve the file
	_, err = io.Copy(w, file)
	return err
}

// WaitForShutdown waits for the server to be fully shutdown
func (s *HTTPServer) WaitForShutdown() {
	<-s.allDone
}
