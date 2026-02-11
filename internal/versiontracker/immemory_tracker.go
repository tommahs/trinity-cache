// Package versiontracker provides utilities to interact with the InMemory tracker
package versiontracker

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/tommahs/trinity-cache/internal/cache"
	"github.com/tommahs/trinity-cache/internal/logger"
)

// InMemoryTracker tracks package versions in memory, backed by a cache manager.
// It provides efficient lookups and updates for package version information.
type InMemoryTracker struct {
	cacheManager cache.CacheManager
	versions     map[string][]string // name -> sorted versions (newest-first)
	mu           sync.RWMutex
}

// NewInMemoryTracker creates a new version tracker backed by a cache manager.
func NewInMemoryTracker(cacheManager cache.CacheManager) (*InMemoryTracker, error) {
	if cacheManager == nil {
		return nil, fmt.Errorf("cache manager cannot be nil")
	}

	return &InMemoryTracker{
		cacheManager: cacheManager,
		versions:     make(map[string][]string),
	}, nil
}

// LatestVersion returns the latest known version for a package name.
func (t *InMemoryTracker) LatestVersion(name string) (string, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	versions, exists := t.versions[name]
	if !exists || len(versions) == 0 {
		return "", fmt.Errorf("no versions known for package %q", name)
	}

	// Versions are stored newest-first
	return versions[0], nil
}

// Update records a new version for the package name.
// If the version already exists, it is not added again.
func (t *InMemoryTracker) Update(name, version string) error {
	if name == "" || version == "" {
		return fmt.Errorf("package name and version cannot be empty")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	versions := t.versions[name]

	// Check if version already exists
	for _, v := range versions {
		if v == version {
			logger.Debug("version already tracked", "name", name, "version", version)
			return nil
		}
	}

	// Add new version and re-sort
	versions = append(versions, version)
	sort.Slice(versions, func(i, j int) bool {
		return versions[i] > versions[j] // newest-first
	})

	t.versions[name] = versions
	logger.Debug("version tracked", "name", name, "version", version)
	return nil
}

// ListVersions returns all known versions for a package, newest-first.
func (t *InMemoryTracker) ListVersions(name string) ([]string, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	versions, exists := t.versions[name]
	if !exists {
		return []string{}, nil
	}

	// Return a copy to prevent external modification
	result := make([]string, len(versions))
	copy(result, versions)
	return result, nil
}

// LoadFromCache synchronizes the tracker with versions available in the cache.
// This should be called during initialization to populate the tracker.
func (t *InMemoryTracker) LoadFromCache() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// This is a placeholder implementation that can be enhanced
	// when we have access to the internal cache structure
	logger.Debug("loading versions from cache")
	return nil
}

// ClearAll clears all tracked versions. Useful for testing or resets.
func (t *InMemoryTracker) ClearAll() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.versions = make(map[string][]string)
	logger.Debug("version tracker cleared")
}

// Stats returns the number of tracked packages and total versions.
func (t *InMemoryTracker) Stats() (packages, totalVersions int) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	packages = len(t.versions)
	for _, versions := range t.versions {
		totalVersions += len(versions)
	}

	return
}

// HasVersion checks if a specific version is known for a package.
func (t *InMemoryTracker) HasVersion(name, version string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	versions, exists := t.versions[name]
	if !exists {
		return false
	}

	for _, v := range versions {
		if v == version {
			return true
		}
	}

	return false
}

// Find locates a package by name (case-insensitive prefix search).
// Returns all package names that match the prefix.
func (t *InMemoryTracker) Find(namePrefix string) []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	prefix := strings.ToLower(namePrefix)
	var matches []string

	for name := range t.versions {
		if strings.Contains(strings.ToLower(name), prefix) {
			matches = append(matches, name)
		}
	}

	sort.Strings(matches)
	return matches
}
