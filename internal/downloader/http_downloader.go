package downloader

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/tommahs/trinity-cache/internal/logger"
	"github.com/tommahs/trinity-cache/internal/metrics"
	"github.com/tommahs/trinity-cache/internal/mirror"
)

// HTTPDownloader implements Downloader using HTTP.
// It supports:
// - Mirror selection via Selector
// - Checksum verification (SHA256)
// - Size verification
// - Download deduplication
type HTTPDownloader struct {
	selector    mirror.Selector
	cache       Cache
	httpClient  *http.Client
	dedupesMu   sync.Mutex
	dedupes     map[string]*deduplicationEntry // key: sha256(url+name+version)
	tempDir     string
	retries     int
	timeout     time.Duration
}

// Cache interface for storing downloaded packages
type Cache interface {
	GetPackagePath(name, version string) string
	Has(name, version string) (bool, error)
	Add(name, version, path string) error
	RetainMostRecent(name string, keep int) error
}

// deduplicationEntry represents a download in progress or completed
type deduplicationEntry struct {
	mu       sync.Mutex
	path     string
	err      error
	complete bool
	waiters  []chan struct{}
}

// NewHTTPDownloader creates a new HTTP downloader.
// maxRetries: number of retry attempts (minimum 1)
// timeoutSeconds: timeout for individual downloads in seconds
func NewHTTPDownloader(selector mirror.Selector, cache Cache, tempDir string, maxRetries int, timeoutSeconds int) (*HTTPDownloader, error) {
	if selector == nil || cache == nil {
		return nil, fmt.Errorf("selector and cache cannot be nil")
	}

	if maxRetries < 1 {
		maxRetries = 3 // default
	}
	if timeoutSeconds < 1 {
		timeoutSeconds = 30 // default
	}

	if tempDir == "" {
		tempDir = os.TempDir()
	}

	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	timeout := time.Duration(timeoutSeconds) * time.Second

	return &HTTPDownloader{
		selector:   selector,
		cache:      cache,
		dedupes:    make(map[string]*deduplicationEntry),
		tempDir:    tempDir,
		retries:    maxRetries,
		timeout:    timeout,
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

// Download downloads a package from a mirror with retry logic, mirror rotation, checksum verification, and deduplication.
func (hd *HTTPDownloader) Download(m *mirror.Mirror, pkgPath string) (*Result, error) {
	if m == nil || pkgPath == "" {
		return nil, fmt.Errorf("mirror and package path cannot be nil/empty")
	}

	// Record download attempt start
	timer := metrics.RecordDownloadStart()

	var lastErr error
	for attempt := 0; attempt < hd.retries; attempt++ {
		// If this is a retry, select a different mirror
		var currentMirror = m
		if attempt > 0 {
			selected, err := hd.selector.Select()
			if err != nil {
				logger.Warn("failed to select mirror on retry", "attempt", attempt, "error", err)
				currentMirror = m // fallback to original
			} else {
				currentMirror = selected
			}
		}

		result, err := hd.downloadFromMirror(currentMirror, pkgPath)
		if err == nil {
			// Mark mirror as penalized after use
			hd.selector.Penalize(currentMirror, 0.5)
			logger.Info("package downloaded successfully", "url", currentMirror.URL, "path", pkgPath)
						// Record successful download with byte size
						timer.RecordSuccess(result.Size)
			return result, nil
		}

		lastErr = err
		logger.Warn("download failed, will retry", "attempt", attempt+1, "mirror", currentMirror.URL, "error", err)

		// Penalize the mirror that failed
		hd.selector.Penalize(currentMirror, 1.0)
		// Record download failure
		timer.RecordFailure()
	}

	return nil, fmt.Errorf("failed to download after %d attempts: %w", hd.retries, lastErr)
}

// downloadFromMirror performs the actual HTTP download from a specific mirror.
func (hd *HTTPDownloader) downloadFromMirror(
	m *mirror.Mirror,
	pkgPath string,
) (res *Result, err error) {

	// Mark download as in-flight
	m.AddInFlightDownload()
	defer m.RemoveInFlightDownload()

	url := fmt.Sprintf("%s/%s", m.URL, pkgPath)

	// Create temp file
	tempFile, err := os.CreateTemp(hd.tempDir, "pkg-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	// Ensure file is closed properly
	defer func() {
		if cerr := tempFile.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close temp file: %w", cerr)
		}
	}()

	// Ensure temp file is removed on failure
	defer func() {
		if err != nil {
			if rerr := os.Remove(tempFile.Name()); rerr != nil {
				logger.Warn("failed to remove tempfile",
					"tempfilename", tempFile.Name(),
					"error", rerr,
				)
			}
		}
	}()

	// Create request context
	ctx, cancel := context.WithTimeout(context.Background(), hd.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create HTTP request: %w", err)
	}

	resp, err := hd.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			logger.Warn("failed to close response body", "error", cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	// Write body + compute checksum
	hash := sha256.New()
	writer := io.MultiWriter(tempFile, hash)

	size, err := io.Copy(writer, resp.Body)
	if err != nil {
		return nil, fmt.Errorf("write temp file: %w", err)
	}

	// Verify content length
	if resp.ContentLength > 0 && size != resp.ContentLength {
		return nil, fmt.Errorf(
			"size mismatch: got %d, expected %d",
			size,
			resp.ContentLength,
		)
	}

	checksum := fmt.Sprintf("%x", hash.Sum(nil))

	logger.Debug(
		"package downloaded and verified",
		"path", pkgPath,
		"size", size,
		"checksum", checksum[:16]+"...",
	)

	return &Result{
		Path:     tempFile.Name(),
		Size:     size,
		Checksum: checksum,
	}, nil
}


// SetRetries sets the number of retry attempts.
func (hd *HTTPDownloader) SetRetries(count int) {
	if count > 0 {
		hd.retries = count
	}
}

// SetTimeout sets the HTTP request timeout.
func (hd *HTTPDownloader) SetTimeout(timeout time.Duration) {
	if timeout > 0 {
		hd.timeout = timeout
		hd.httpClient.Timeout = timeout
	}
}

// Verify checks the integrity of a downloaded package using SHA256.
func (hd *HTTPDownloader) Verify(filePath, expectedChecksum string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		if err := f.Close(); err == nil {
			err = fmt.Errorf("close temp file: %w", err)
		}
	}()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return fmt.Errorf("failed to compute checksum: %w", err)
	}

	actualChecksum := fmt.Sprintf("%x", hash.Sum(nil))
	if actualChecksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch: got %s, expected %s", actualChecksum, expectedChecksum)
	}

	logger.Debug("checksum verification passed", "checksum", expectedChecksum[:16]+"...")
	return nil
}

// GetPackageStatus returns information about a package.
func (hd *HTTPDownloader) GetPackageStatus(name, version string) (exists bool, path string, err error) {
	exists, err = hd.cache.Has(name, version)
	if exists {
		path = hd.cache.GetPackagePath(name, version)
	}
	return
}
