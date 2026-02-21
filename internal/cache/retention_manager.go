package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tommahs/trinity-cache/internal/logger"
	"github.com/tommahs/trinity-cache/internal/metrics"
)

// RetentionManager enforces version retention policies on the cache.
// Currently implements: keep 2 most recent versions.
type RetentionManager struct {
	cache           CacheManager
	retentionCount  int
	mu              sync.Mutex
	stopChan        chan struct{}
	running         bool
	ticker          *time.Ticker
	lastCleanupTime time.Time
}

// NewRetentionManager creates a retention manager with a default of 2 versions.
func NewRetentionManager(cache CacheManager) *RetentionManager {
	return &RetentionManager{
		cache:          cache,
		retentionCount: 2,
	}
}

// SetRetentionCount sets how many versions to keep (default: 2).
func (rm *RetentionManager) SetRetentionCount(count int) error {
	if count < 1 {
		return fmt.Errorf("retention count must be at least 1")
	}
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.retentionCount = count
	logger.Debug("retention count updated", "count", count)
	return nil
}

// EnforceNow immediately enforces retention on all packages in the cache.
func (rm *RetentionManager) EnforceNow() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	filesystemCache, ok := rm.cache.(*FilesystemCache)
	if !ok {
		return fmt.Errorf("cache must be a FilesystemCache for retention enforcement")
	}

	storagePath := filesystemCache.GetStoragePath()
	entries, err := os.ReadDir(storagePath)
	if err != nil {
		return fmt.Errorf("failed to read cache storage: %w", err)
	}

	removedCount := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		packageName := entry.Name()
		packageDir := filepath.Join(storagePath, packageName)

		// Count package files
		pkgEntries, err := os.ReadDir(packageDir)
		if err != nil {
			logger.Warn("failed to read package directory", "path", packageDir, "error", err)
			continue
		}

		pkgCount := 0
		for _, e := range pkgEntries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".pkg") {
				pkgCount++
			}
		}

		// Enforce retention
		if pkgCount > rm.retentionCount {
			if err := rm.cache.RetainMostRecent(packageName, rm.retentionCount); err != nil {
				logger.Error("failed to enforce retention", "package", packageName, "error", err)
			} else {
				removed := pkgCount - rm.retentionCount
				removedCount += removed
				// record metrics for removed versions/packages
				for i := 0; i < removed; i++ {
					metrics.RecordVersionRemoved()
				}
				// Record package-level removal once per package if any versions removed
				if removed > 0 {
					metrics.RecordPackageRemoved()
				}
			}
		}
	}

	rm.lastCleanupTime = time.Now()
	if removedCount > 0 {
		logger.Info("retention enforcement completed", "packages_processed", len(entries), "versions_removed", removedCount)
	}

	return nil
}

// StartPeriodicEnforcement starts a background goroutine that periodically enforces retention.
// The interval controls how often enforcement runs.
func (rm *RetentionManager) StartPeriodicEnforcement(interval time.Duration) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.running {
		logger.Debug("retention enforcement already running")
		return nil
	}

	if interval < 1*time.Minute {
		interval = 5 * time.Minute // minimum 5 minutes
		logger.Debug("retention interval adjusted to minimum", "interval", interval.String())
	}

	rm.stopChan = make(chan struct{})
	rm.ticker = time.NewTicker(interval)
	rm.running = true

	logger.Info("retention enforcement started", "interval", interval.String())

	go rm.enforcementLoop()
	return nil
}

// enforcementLoop runs the periodic retention enforcement.
func (rm *RetentionManager) enforcementLoop() {
	defer func() {
		rm.mu.Lock()
		rm.running = false
		rm.mu.Unlock()
	}()

	for {
		select {
		case <-rm.stopChan:
			logger.Debug("retention enforcement stopping")
			return
		case <-rm.ticker.C:
			rm.mu.Lock()
			err := rm.EnforceNow()
			rm.mu.Unlock()

			if err != nil {
				logger.Error("retention enforcement error", "error", err)
			}
		}
	}
}

// Stop stops the periodic retention enforcement.
func (rm *RetentionManager) Stop() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if !rm.running || rm.stopChan == nil {
		return
	}

	close(rm.stopChan)
	if rm.ticker != nil {
		rm.ticker.Stop()
	}

	logger.Debug("retention enforcement stopped")
}

// IsRunning returns whether periodic enforcement is active.
func (rm *RetentionManager) IsRunning() bool {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	return rm.running
}

// LastCleanupTime returns when retention was last enforced.
func (rm *RetentionManager) LastCleanupTime() time.Time {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	return rm.lastCleanupTime
}

// GetRetentionCount returns the current retention count.
func (rm *RetentionManager) GetRetentionCount() int {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	return rm.retentionCount
}
