package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	mux.HandleFunc("/", s.handlePacmanRequest)          // Primary pacman route
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
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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

// handlePacmanRequest handles Arch Linux pacman package requests
// Format: /{repo}/os/{arch}/{package-version}.pkg.tar.zst
// Example: /core/os/x86_64/linux-6.7.1-1-x86_64.pkg.tar.zst
func (s *HTTPServer) handlePacmanRequest(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt32(&s.activeRequests, 1)
	defer atomic.AddInt32(&s.activeRequests, -1)

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract the path after the host
	path := r.URL.Path
	if path == "/" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Parse path: /{repo}/os/{arch}/{filename}.pkg.tar.zst
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 || parts[1] != "os" || !strings.HasSuffix(parts[len(parts)-1], ".pkg.tar.zst") {
		// Not a pacman request
		w.WriteHeader(http.StatusNotFound)
		return
	}

	repo := parts[0]
	arch := parts[2]
	filename := parts[len(parts)-1]

	logger.Info("pacman request", "repo", repo, "arch", arch, "filename", filename)

	// Parse package name and version from filename
	// Format: {package}-{version}-{arch}.pkg.tar.zst
	pkgName, version, err := s.parsePackageName(filename, arch)
	if err != nil {
		logger.Warn("failed to parse package", "filename", filename, "error", err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Invalid package format")
		return
	}

	// Check if package exists in cache
	cacheExists, _ := s.cacheManager.Has(pkgName, version)
	if cacheExists {
		logger.Debug("serving from cache", "package", pkgName, "version", version)
		metrics.RecordCacheHit()
		pkgPath := s.getPackagePath(pkgName, version, filename)
		if err := s.ServePackage(w, pkgPath); err != nil {
			logger.Error("failed to serve cached package", "path", pkgPath, "error", err)
		}
		return
	}

	// Cache miss - try to download from upstream mirror
	logger.Info("cache miss, fetching from upstream", "package", pkgName, "version", version)
	metrics.RecordCacheMiss()

	// Construct paths
	upstreamPath := fmt.Sprintf("%s/os/%s/%s", repo, arch, filename) // For upstream mirror
	localPath := s.getPackagePath(pkgName, version, filename)         // For local cache

	// Try to fetch the package
	if s.fetchManager != nil {
		result, err := s.fetchManager.FetchVersion(pkgName, version, upstreamPath)
		if err == nil && result != nil {
			// Move downloaded file to cache location
			if err := s.moveFileToCache(result.Path, localPath); err != nil {
				logger.Error("failed to move file to cache", "from", result.Path, "to", localPath, "error", err)
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "Failed to cache package: %s", err.Error())
				return
			}

			logger.Info("package cached successfully", "package", pkgName, "version", version, "size", result.Size)
			metrics.RecordCacheHit() // Record as hit after successful fetch and cache

			// Serve from cache now
			if err := s.ServePackage(w, localPath); err != nil {
				logger.Error("failed to serve package from cache", "path", localPath, "error", err)
			}
			return
		}

		if err != nil {
			logger.Error("failed to fetch package", "package", pkgName, "version", version, "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Failed to fetch package: %s", err.Error())
			return
		}
	}

	// No fetch manager available
	logger.Warn("fetch manager not available, cannot fetch missing package", "package", pkgName, "version", version)
	w.WriteHeader(http.StatusServiceUnavailable)
	fmt.Fprintf(w, "Package not in cache and fetch unavailable")
}

// parsePackageName extracts package name and version from filename
// Filename format: {package}-{version}-{arch}.pkg.tar.zst
// Example: linux-6.7.1-1-x86_64.pkg.tar.zst -> (linux, 6.7.1-1)
func (s *HTTPServer) parsePackageName(filename, arch string) (string, string, error) {
	// Remove extension
	baseName := strings.TrimSuffix(filename, ".pkg.tar.zst")

	// Remove arch suffix
	archSuffix := "-" + arch
	if !strings.HasSuffix(baseName, archSuffix) {
		return "", "", fmt.Errorf("filename doesn't end with expected arch suffix")
	}

	baseName = strings.TrimSuffix(baseName, archSuffix)

	// Find where package name ends and version begins
	// Version typically starts with a digit, and package name may contain hyphens
	// Search from the end for the last occurrence of hyphen followed by version-like pattern
	parts := strings.Split(baseName, "-")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid package format")
	}

	// Try to find where version starts (typically a digit)
	// Work backwards to find the version separator
	for i := len(parts) - 1; i > 0; i-- {
		if len(parts[i]) > 0 && (parts[i][0] >= '0' && parts[i][0] <= '9') {
			// Found potential version start
			pkgName := strings.Join(parts[:i], "-")
			version := strings.Join(parts[i:], "-")

			if pkgName != "" && version != "" {
				return pkgName, version, nil
			}
		}
	}

	return "", "", fmt.Errorf("could not parse package name and version")
}

// getPackagePath constructs the path where a package should be stored
func (s *HTTPServer) getPackagePath(pkgName, version, filename string) string {
	// For filesystem cache, construct path like: storage_path/package_name/filename
	if fc, ok := s.cacheManager.(*cache.FilesystemCache); ok {
		return fc.GetPackagePath(pkgName, version)
	}
	return fmt.Sprintf("/cache/%s/%s", pkgName, filename)
}

// moveFileToCache moves a downloaded file from temp location to the cache directory
func (s *HTTPServer) moveFileToCache(tempPath, cachePath string) error {
	if tempPath == "" || cachePath == "" {
		return fmt.Errorf("temp and cache paths cannot be empty")
	}

	// Create cache directory if it doesn't exist
	cacheDir := fmt.Sprintf("%s/%s", filepath.Dir(cachePath), "")
	cacheDir = filepath.Dir(cachePath)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Move temp file to cache
	if err := os.Rename(tempPath, cachePath); err != nil {
		logger.Debug("rename failed, trying copy+delete", "from", tempPath, "to", cachePath, "error", err)
		// Fallback: copy then delete
		if err := copyFile(tempPath, cachePath); err != nil {
			return fmt.Errorf("failed to copy file to cache: %w", err)
		}
		if err := os.Remove(tempPath); err != nil {
			logger.Warn("failed to remove temp file after copy", "path", tempPath, "error", err)
		}
	}

	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	dest, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dest.Close()

	_, err = io.Copy(dest, source)
	return err
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
		"checksum":  result.Checksum,
		"path":      result.Path,
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
