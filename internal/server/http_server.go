// Package server implements the HTTPServer. It contains utilities to handle packages, interact with cache and handle stats and metrics.
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
	cacheManager   cache.CacheManager
	fetchManager   *downloader.FetchManager
	httpServer     *http.Server
	mu             sync.Mutex
	running        bool
	shutdownChan   chan struct{}
	allDone        chan struct{}
	activeRequests int32
}

// NewHTTPServer creates a new HTTP server for package serving
// readTimeoutSec and writeTimeoutSec are in seconds
func NewHTTPServer(cache cache.CacheManager, addr string, readTimeoutSec, writeTimeoutSec int) (*HTTPServer, error) {
	if cache == nil {
		return nil, fmt.Errorf("cache manager cannot be nil")
	}

	if addr == "" {
		addr = ":8080"
	}

	if readTimeoutSec < 1 {
		readTimeoutSec = 30
	}
	if writeTimeoutSec < 1 {
		writeTimeoutSec = 30
	}

	s := &HTTPServer{
		cacheManager: cache,
		shutdownChan: make(chan struct{}),
		allDone:      make(chan struct{}),
	}

	// Create HTTP server with routes
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handlePacmanRequest) // Primary pacman route
	mux.HandleFunc("/api/v1/packages/", s.handlePackageRequest)
	mux.HandleFunc("/api/v1/fetch/", s.handleFetchRequest)
	mux.HandleFunc("/api/v1/stats", s.handleStatsRequest)
	mux.HandleFunc("/api/v1/metrics", s.handleMetrics)         // Prometheus format by default
	mux.HandleFunc("/metrics", s.handleMetrics)                // Prometheus format (standard path)
	mux.HandleFunc("/metrics/summary", s.handleMetricsSummary) // Human-readable summary
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/healthz", s.handleHealth) // Kubernetes-compatible health endpoint

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  time.Duration(readTimeoutSec) * time.Second,
		WriteTimeout: time.Duration(writeTimeoutSec) * time.Second,
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
	// 
	if ctx == nil {
		//nolint:contextcheck // allow default background context when ctx is nil
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
		if err := s.httpServer.Close(); err !=nil {
			logger.Error("error closing http server", "error", err)
		}
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

// pacmanRequestInfo holds parsed pacman request information
type pacmanRequestInfo struct {
	repo       string
	arch       string
	filename   string
	isRepoFile bool
	pkgName    string
	version    string
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

	// Parse and validate request
	reqInfo, err := s.parsePacmanRequest(r.URL.Path)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Route to appropriate handler
	if reqInfo.isRepoFile {
		s.handlePacmanRepoFile(w, reqInfo)
	} else {
		s.handlePacmanPackageFile(w, reqInfo)
	}
}

// parsePacmanRequest extracts and validates pacman request path components
func (s *HTTPServer) parsePacmanRequest(path string) (*pacmanRequestInfo, error) {
	if path == "/" {
		return nil, fmt.Errorf("empty path")
	}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 1 {
		return nil, fmt.Errorf("invalid path")
	}

	info := &pacmanRequestInfo{
		repo:     parts[0],
		filename: parts[len(parts)-1],
	}

	// Determine if this is a package file or repo file
	info.isRepoFile = !strings.HasSuffix(info.filename, ".pkg.tar.zst")

	if info.isRepoFile {
		// Repo files: /{repo}/os/{arch}/{filename}
		if len(parts) < 3 || parts[1] != "os" {
			return nil, fmt.Errorf("invalid repo file path")
		}
		info.arch = parts[2]
	} else {
		// Package files: /{repo}/os/{arch}/{filename}.pkg.tar.zst
		if len(parts) < 4 || parts[1] != "os" {
			return nil, fmt.Errorf("invalid package path")
		}
		info.arch = parts[2]

		// Parse package name and version
		pkgName, version, err := s.parsePackageName(info.filename, info.arch)
		if err != nil {
			return nil, fmt.Errorf("invalid package format: %w", err)
		}
		info.pkgName = pkgName
		info.version = version
	}

	return info, nil
}

// handlePacmanRepoFile handles repository metadata files (e.g., core.db)
func (s *HTTPServer) handlePacmanRepoFile(w http.ResponseWriter, info *pacmanRequestInfo) {
	fc, ok := s.cacheManager.(*cache.FilesystemCache)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	repoFile := filepath.Join(fc.GetStoragePath(), info.repo, "os", info.arch, info.filename)

	// Try to serve from cache
	if s.tryServeRepoFileFromCache(w, repoFile, info.repo, info.filename) {
		return
	}

	// Fetch from upstream if not in cache
	s.fetchAndServeRepoFile(w, info, repoFile, fc)
}

// tryServeRepoFileFromCache attempts to serve a repo file from cache
func (s *HTTPServer) tryServeRepoFileFromCache(w http.ResponseWriter, repoFile, repo, filename string) bool {
	if _, err := os.Stat(repoFile); err == nil {
		logger.Info("serving repo file", "repo", repo, "filename", filename)
		metrics.RecordCacheHit()
		if servererr := s.ServePackage(w, repoFile); servererr != nil {
			logger.Error("failed to serve repo file", "path", repoFile, "error", servererr)
		}
		return true
	}
	return false
}

// fetchAndServeRepoFile fetches a repo file from upstream and serves it
func (s *HTTPServer) fetchAndServeRepoFile(w http.ResponseWriter, info *pacmanRequestInfo, repoFile string, fc *cache.FilesystemCache) {
	if s.fetchManager == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	upstreamPath := fmt.Sprintf("%s/os/%s/%s", info.repo, info.arch, info.filename)
	result, err := s.fetchManager.FetchVersion(info.repo, info.filename, upstreamPath)

	if err != nil {
		logger.Error("failed to fetch repo file", "repo", info.repo, "filename", info.filename, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		////nolint:errcheck
		fmt.Fprintf(w, "Failed to fetch repo file: %s", err.Error())
		return
	}

	if result == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Cache the fetched file
	if err := s.cacheRepoFile(result.Path, repoFile, info, fc); err != nil {
		logger.Error("failed to cache repo file", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := fmt.Fprintf(w, "Failed to cache repo file: %s", err.Error()); err != nil {
			logger.Error("failed to cache repo file ", "error", err)
		}
		
		return
	}

	logger.Info("repo file cached successfully", "repo", info.repo, "filename", info.filename, "size", result.Size)
	metrics.RecordCacheHit()

	if err := s.ServePackage(w, repoFile); err != nil {
		logger.Error("failed to serve repo file from cache", "path", repoFile, "error", err)
	}
}

// cacheRepoFile stores a repo file in the cache
func (s *HTTPServer) cacheRepoFile(sourcePath, destPath string, info *pacmanRequestInfo, fc *cache.FilesystemCache) error {
	if fc != nil {
		_, err := fc.PutRepoFile(info.repo, info.arch, info.filename, sourcePath)
		return err
	}
	return s.moveFileToCache(sourcePath, destPath)
}

// handlePacmanPackageFile handles package file requests
func (s *HTTPServer) handlePacmanPackageFile(w http.ResponseWriter, info *pacmanRequestInfo) {
	logger.Info("pacman request", "repo", info.repo, "arch", info.arch, "filename", info.filename)

	fc, ok := s.cacheManager.(*cache.FilesystemCache)
	if ok {
		localPath := filepath.Join(fc.GetStoragePath(), info.repo, "os", info.arch, info.filename)
		if s.tryServePackageFromCache(w, localPath, info.pkgName, info.version) {
			return
		}
		s.fetchAndServePackage(w, info, localPath, fc)
	} else {
		s.handlePackageFallbackCache(w, info)
	}
}

// tryServePackageFromCache attempts to serve a package from cache
func (s *HTTPServer) tryServePackageFromCache(w http.ResponseWriter, localPath, pkgName, version string) bool {
	if _, err := os.Stat(localPath); err == nil {
		logger.Debug("serving from cache", "package", pkgName, "version", version)
		metrics.RecordCacheHit()
		if err := s.ServePackage(w, localPath); err != nil {
			logger.Error("failed to serve cached package", "path", localPath, "error", err)
		}
		return true
	}
	return false
}

// fetchAndServePackage fetches a package from upstream and serves it
func (s *HTTPServer) fetchAndServePackage(w http.ResponseWriter, info *pacmanRequestInfo, localPath string, fc *cache.FilesystemCache) {
	logger.Info("cache miss, fetching from upstream", "package", info.pkgName, "version", info.version)
	metrics.RecordCacheMiss()

	if s.fetchManager == nil {
		s.respondFetchUnavailable(w, info.pkgName, info.version)
		return
	}

	upstreamPath := fmt.Sprintf("%s/os/%s/%s", info.repo, info.arch, info.filename)
	result, err := s.fetchManager.FetchVersion(info.pkgName, info.version, upstreamPath)

	if err != nil {
		logger.Error("failed to fetch package", "package", info.pkgName, "version", info.version, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		if _, writeErr := fmt.Fprintf(w, "Failed to fetch package: %s", err.Error()); writeErr != nil {
			logger.Warn("failed to write HTTP error response", "error", writeErr)
		}
		return
	}

	if result == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Cache the fetched package
	if err := s.cachePackageFile(result.Path, localPath, info, fc); err != nil {
		logger.Error("failed to cache package", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		if _, writeErr := fmt.Fprintf(w, "Failed to cache package: %s", err.Error()); writeErr != nil {
			logger.Warn("failed to write HTTP error response", "error", writeErr)
		}
		return
	}

	logger.Info("package cached successfully", "package", info.pkgName, "version", info.version, "size", result.Size)
	metrics.RecordCacheHit()

	if err := s.ServePackage(w, localPath); err != nil {
		logger.Error("failed to serve package from cache", "path", localPath, "error", err)
	}
}

// cachePackageFile stores a package file in the cache
func (s *HTTPServer) cachePackageFile(sourcePath, destPath string, info *pacmanRequestInfo, fc *cache.FilesystemCache) error {
	if fc != nil {
		_, err := fc.PutPackageFile(info.repo, info.arch, info.filename, sourcePath)
		return err
	}
	return s.moveFileToCache(sourcePath, destPath)
}

// handlePackageFallbackCache handles package requests when FilesystemCache is not available
func (s *HTTPServer) handlePackageFallbackCache(w http.ResponseWriter, info *pacmanRequestInfo) {
	cacheExists, _ := s.cacheManager.Has(info.pkgName, info.version)
	if cacheExists {
		logger.Debug("serving from cache (fallback)", "package", info.pkgName, "version", info.version)
		metrics.RecordCacheHit()
		localPath := s.getPackagePath(info.repo, info.arch, info.filename)
		if err := s.ServePackage(w, localPath); err != nil {
			logger.Error("failed to serve cached package", "path", localPath, "error", err)
		}
		return
	}

	s.fetchPackageFallback(w, info)
}

// fetchPackageFallback fetches and serves a package using fallback cache
func (s *HTTPServer) fetchPackageFallback(w http.ResponseWriter, info *pacmanRequestInfo) {
	logger.Info("cache miss, fetching from upstream", "package", info.pkgName, "version", info.version)
	metrics.RecordCacheMiss()

	if s.fetchManager == nil {
		s.respondFetchUnavailable(w, info.pkgName, info.version)
		return
	}

	localPath := s.getPackagePath(info.repo, info.arch, info.filename)
	upstreamPath := fmt.Sprintf("%s/os/%s/%s", info.repo, info.arch, info.filename)

	result, err := s.fetchManager.FetchVersion(info.pkgName, info.version, upstreamPath)
	if err != nil {
		logger.Error("failed to fetch package", "package", info.pkgName, "version", info.version, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
			// Check error from writing the response
		if _, writeErr := fmt.Fprintf(w, "Failed to fetch package: %s", err.Error()); writeErr != nil {
			logger.Warn("failed to write HTTP error response", "error", writeErr)
		}
		return
	}

	if result == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if err := s.moveFileToCache(result.Path, localPath); err != nil {
		logger.Error("failed to move file to cache", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		
		if _, writeErr := fmt.Fprintf(w, "Failed to cache package: %s", err.Error()); writeErr != nil {
			logger.Warn("failed to write HTTP error response", "error", writeErr)
		}
		return
	}

	logger.Info("package cached successfully (fallback)", "package", info.pkgName, "version", info.version, "size", result.Size)
	metrics.RecordCacheHit()

	if err := s.ServePackage(w, localPath); err != nil {
		logger.Error("failed to serve package from cache", "path", localPath, "error", err)
	}
}

// respondFetchUnavailable sends a 503 response when fetch manager is unavailable
func (s *HTTPServer) respondFetchUnavailable(w http.ResponseWriter, pkgName, version string) {
	logger.Warn("fetch manager not available, cannot fetch missing package", "package", pkgName, "version", version)
	w.WriteHeader(http.StatusServiceUnavailable)
	
	if _, writeErr := fmt.Fprintf(w, "Package not in cache and fetch unavailable"); writeErr != nil {
		logger.Warn("failed to write HTTP error response", "error", writeErr)
	}
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
func (s *HTTPServer) getPackagePath(repo, arch, filename string) string {
	// For filesystem cache, construct path like: storage_path/<repo>/os/<arch>/<filename>
	if fc, ok := s.cacheManager.(*cache.FilesystemCache); ok {
		return filepath.Join(fc.GetStoragePath(), repo, "os", arch, filepath.Base(filename))
	}
	return fmt.Sprintf("/cache/%s/%s", repo, filename)
}

// moveFileToCache moves a downloaded file from temp location to the cache directory
func (s *HTTPServer) moveFileToCache(tempPath, cachePath string) error {
	if tempPath == "" || cachePath == "" {
		return fmt.Errorf("temp and cache paths cannot be empty")
	}

	// Create cache directory if it doesn't exist
	// cacheDir := fmt.Sprintf("%s/%s", filepath.Dir(cachePath), "")
	cacheDir := filepath.Dir(cachePath)
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
		// Ensure file is closed properly
	defer func() {
		if closeErr := source.Close(); closeErr != nil {
			err = fmt.Errorf("close temp file: %w", closeErr)
		}
	}()

	dest, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := dest.Close(); closeErr == nil {
			err = fmt.Errorf("close temp file: %w", closeErr)
		}
	}()

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
		if _, writeErr := fmt.Fprintf(w, `{"error":"package path required"}`); writeErr != nil {
			logger.Warn("failed to write HTTP error response", "error", writeErr)
		}
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
		if _, writeErr := fmt.Fprintf(w, `{"error":"fetch manager not available"}`); writeErr != nil {
			logger.Warn("failed to write HTTP error response", "error", writeErr)
		}		
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
		if _, writeErr := fmt.Fprintf(w, `{"error":"package name and version required"}`); writeErr != nil {
			logger.Warn("failed to write HTTP error response", "error", writeErr)
		}
		return
	}

	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if _, writeErr := fmt.Fprintf(w, `{"error":"invalid package path"}`); writeErr != nil {
			logger.Warn("failed to write HTTP error response", "error", writeErr)
		}		
		
		return
	}

	name := parts[0]
	version := parts[1]

	if name == "" || version == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if _, writeErr := fmt.Fprintf(w, `{"error":"name and version required"}`); writeErr != nil {
			logger.Warn("failed to write HTTP error response", "error", writeErr)
		}		
		
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
		if _, writeErr := fmt.Fprintf(w, `{"error":"fetch failed: %s"}`, err.Error()); writeErr != nil {
			logger.Warn("failed to write HTTP error response", "error", writeErr)
		}	
		
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
			"total":           snapshot.Downloads.Total,
			"successful":      snapshot.Downloads.Successful,
			"failed":          snapshot.Downloads.Failed,
			"bytes_total":     snapshot.Downloads.BytesTotal,
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
	if err := json.NewEncoder(w).Encode(stats); err != nil {
	logger.Warn("failed to encode HTTP response", "error", err)
}

}

// handleHealth returns server health status
func (s *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	activeRequests := atomic.LoadInt32(&s.activeRequests)

	health := map[string]interface{}{
		"status":          "ok",
		"running":         s.IsRunning(),
		"active_requests": activeRequests,
		"timestamp":       time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(health); err != nil {
	logger.Warn("failed to encode HTTP response", "error", err)
}
}

// handleMetrics returns metrics in Prometheus or JSON format
// Supports query parameter ?format=json or Accept header for format negotiation
func (s *HTTPServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Check for format query parameter
	format := r.URL.Query().Get("format")
	if format == "" {
		// Check Accept header for prometheus format
		acceptHeader := r.Header.Get("Accept")
		if strings.Contains(acceptHeader, "application/vnd.google.protobuf") ||
			strings.Contains(acceptHeader, "text/plain") {
			format = "prometheus"
		}
	}

	// Default to prometheus format
	if format == "" {
		format = "prometheus"
	}

	if format == "json" {
		// Return JSON format for backward compatibility
		metricsData := metrics.GetMetricsJSON()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(metricsData)
		if err := json.NewEncoder(w).Encode(metricsData); err != nil {
			logger.Warn("failed to encode HTTP response", "error", err)
		}
		return
	}

	// Return Prometheus format (0.0.4)
	pm := metrics.NewPrometheusMetrics(nil)
	prometheusData := pm.Export()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, writeErr := fmt.Fprint(w, prometheusData); writeErr != nil {
		logger.Warn("failed to write HTTP prometheusData response", "error", writeErr)
	}
	
}

// handleMetricsSummary serves a human-readable metrics summary
func (s *HTTPServer) handleMetricsSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	metricsData := metrics.NewPrometheusMetrics(metrics.GetGlobalMetrics()).ExportSummary()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, writeErr := fmt.Fprint(w, metricsData); writeErr != nil {
		logger.Warn("failed to write HTTP metricsData response", "error", writeErr)
	}
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
	// Ensure file is closed properly
	defer func() {
		if closeErr := file.Close(); closeErr == nil {
			err = fmt.Errorf("close temp file: %w", closeErr)
		}
	}()

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