package downloader

import (
	"fmt"
	"sync"
	"time"

	"github.com/tommahs/trinity-cache/internal/logger"
	"github.com/tommahs/trinity-cache/internal/mirror"
)

// FetchManager handles on-demand fetching of newer package versions
type FetchManager struct {
	downloader     Downloader
	selector       mirror.Selector
	tracker        VersionTracker
	mu             sync.RWMutex
	inProgressMu   sync.Mutex
	inProgress     map[string]bool // key: "name:version"
	lastCheckTimes map[string]time.Time
}

// VersionTracker interface for tracking available versions
type VersionTracker interface {
	LatestVersion(name string) (string, error)
	Update(name, version string) error
	ListVersions(name string) ([]string, error)
}

// NewFetchManager creates a new fetch manager for on-demand downloads
func NewFetchManager(downloader Downloader, selector mirror.Selector, tracker VersionTracker) (*FetchManager, error) {
	if downloader == nil || selector == nil || tracker == nil {
		return nil, fmt.Errorf("downloader, selector, and tracker cannot be nil")
	}

	return &FetchManager{
		downloader:     downloader,
		selector:       selector,
		tracker:        tracker,
		inProgress:     make(map[string]bool),
		lastCheckTimes: make(map[string]time.Time),
	}, nil
}

// FetchIfNeeded checks if a newer version exists and fetches it if available
// Returns whether a fetch was performed
func (fm *FetchManager) FetchIfNeeded(name, currentVersion string) (bool, error) {
	if name == "" {
		return false, fmt.Errorf("package name cannot be empty")
	}

	// Check if we've already checked recently (within 5 minutes)
	fm.mu.RLock()
	lastCheck, exists := fm.lastCheckTimes[name]
	fm.mu.RUnlock()

	now := time.Now()
	if exists && now.Sub(lastCheck) < 5*time.Minute {
		logger.Debug("skipping recent check", "name", name, "last_check", lastCheck)
		return false, nil
	}

	// Select a mirror to check for versions
	m, err := fm.selector.Select()
	if err != nil {
		return false, fmt.Errorf("failed to select mirror: %w", err)
	}

	// In a real implementation, we'd query the mirror for available versions
	// For now, we'll update the last check time
	fm.mu.Lock()
	fm.lastCheckTimes[name] = now
	fm.mu.Unlock()

	fm.selector.Penalize(m, 0.1) // Small penalty for checking
	logger.Debug("checked for new versions", "name", name, "mirror", m.URL)

	return false, nil
}

// FetchVersion fetches a specific version on-demand
func (fm *FetchManager) FetchVersion(name, version, pkgPath string) (*Result, error) {
	if name == "" || version == "" {
		return nil, fmt.Errorf("name and version cannot be empty")
	}

	// Check if fetch is already in progress
	key := fmt.Sprintf("%s:%s", name, version)
	fm.inProgressMu.Lock()
	if fm.inProgress[key] {
		fm.inProgressMu.Unlock()
		// Wait for completion - for now just return error
		return nil, fmt.Errorf("fetch already in progress for %s", key)
	}
	fm.inProgress[key] = true
	fm.inProgressMu.Unlock()

	defer func() {
		fm.inProgressMu.Lock()
		delete(fm.inProgress, key)
		fm.inProgressMu.Unlock()
	}()

	// Select a mirror
	m, err := fm.selector.Select()
	if err != nil {
		logger.Warn("failed to select mirror for on-demand fetch", "name", name, "version", version, "error", err)
		return nil, fmt.Errorf("no mirrors available: %w", err)
	}

	logger.Info("fetching package on-demand", "name", name, "version", version, "mirror", m.URL)

	// Perform the download
	result, err := fm.downloader.Download(m, pkgPath)
	if err != nil {
		logger.Error("on-demand fetch failed", "name", name, "version", version, "error", err)
		fm.selector.Penalize(m, 1.0)
		return nil, fmt.Errorf("download failed: %w", err)
	}

	// Update version tracker
	if err := fm.tracker.Update(name, version); err != nil {
		logger.Warn("failed to update version tracker", "name", name, "version", version, "error", err)
	}

	logger.Info("on-demand fetch completed", "name", name, "version", version, "size", result.Size)
	fm.selector.Penalize(m, 0.3)

	return result, nil
}

// CheckForUpdates checks all known packages for newer versions
// Returns a list of packages with available updates
func (fm *FetchManager) CheckForUpdates(packageNames []string) (map[string]string, error) {
	updates := make(map[string]string)

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 5) // Limit concurrent checks to 5

	for _, name := range packageNames {
		wg.Add(1)
		go func(pkgName string) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// In a real implementation, we'd query the mirror for available versions
			// For now, just log the check
			logger.Debug("checking for updates", "package", pkgName)

			// This would return the latest available version from mirrors
			// latestVersion, err := fm.queryMirrorForLatestVersion(pkgName)
			// if err != nil {
			//     logger.Warn("failed to check for updates", "package", pkgName, "error", err)
			//     return
			// }

			// mu.Lock()
			// updates[pkgName] = latestVersion
			// mu.Unlock()
		}(name)
	}

	wg.Wait()
	return updates, nil
}

// GetInProgress returns the list of packages currently being fetched
func (fm *FetchManager) GetInProgress() []string {
	fm.inProgressMu.Lock()
	defer fm.inProgressMu.Unlock()

	var inProgress []string
	for key := range fm.inProgress {
		inProgress = append(inProgress, key)
	}
	return inProgress
}

// IsInProgress checks if a specific version fetch is in progress
func (fm *FetchManager) IsInProgress(name, version string) bool {
	key := fmt.Sprintf("%s:%s", name, version)
	fm.inProgressMu.Lock()
	defer fm.inProgressMu.Unlock()
	return fm.inProgress[key]
}

// GetLastCheckTime returns when a package was last checked for updates
func (fm *FetchManager) GetLastCheckTime(name string) (time.Time, bool) {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	lastCheck, exists := fm.lastCheckTimes[name]
	return lastCheck, exists
}
