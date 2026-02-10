package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tommahs/trinity-cache/internal/logger"
)

// FilesystemCache implements CacheManager using the local filesystem.
// The cache layout is:
//   storage_path/
//     package_name/
//       package_name-version.pkg (actual package file)
//       metadata.json (version metadata)
type FilesystemCache struct {
	storagePath string
}

// PackageMetadata holds version information for a package.
type PackageMetadata struct {
	Name     string `json:"name"`
	Versions []struct {
		Version string `json:"version"`
		Path    string `json:"path"`
	} `json:"versions"`
}

// NewFilesystemCache creates a new filesystem-based cache manager.
func NewFilesystemCache(storagePath string) (*FilesystemCache, error) {
	// Ensure storage path exists
	if err := os.MkdirAll(storagePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage path: %w", err)
	}

	logger.Debug("filesystem cache initialized", "path", storagePath)
	return &FilesystemCache{
		storagePath: storagePath,
	}, nil
}

// Has returns true if the given package name/version exists in cache.
func (fc *FilesystemCache) Has(name, version string) (bool, error) {
	packageDir := filepath.Join(fc.storagePath, name)
	_, err := os.Stat(packageDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	// Check if the package file exists
	pkgFile := filepath.Join(packageDir, fmt.Sprintf("%s-%s.pkg", name, version))
	_, err = os.Stat(pkgFile)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// GetLatest returns the most recent PackageVersion for name.
func (fc *FilesystemCache) GetLatest(name string) (*PackageVersion, error) {
	versions, err := fc.ListVersions(name)
	if err != nil {
		return nil, err
	}

	if len(versions) == 0 {
		return nil, nil
	}

	// Versions are already sorted newest-first by ListVersions
	return versions[0], nil
}

// Add inserts a new package version into the cache.
func (fc *FilesystemCache) Add(p *PackageVersion) error {
	if p == nil || p.Name == "" || p.Version == "" {
		return fmt.Errorf("invalid package: missing name or version")
	}

	packageDir := filepath.Join(fc.storagePath, p.Name)
	if err := os.MkdirAll(packageDir, 0755); err != nil {
		return fmt.Errorf("failed to create package directory: %w", err)
	}

	// Ensure the package exists at the expected path
	expectedPath := filepath.Join(packageDir, fmt.Sprintf("%s-%s.pkg", p.Name, p.Version))
	if p.Path != expectedPath {
		// Try to move or link the file if it's elsewhere
		if _, err := os.Stat(p.Path); err == nil {
			// File exists at p.Path, but we need it at expectedPath
			if err := os.Rename(p.Path, expectedPath); err != nil {
				return fmt.Errorf("failed to move package file: %w", err)
			}
		}
	}

	logger.Debug("package added to cache", "name", p.Name, "version", p.Version, "path", expectedPath)
	return nil
}

// ListVersions lists versions known for a package name, ordered newest-first.
// The sorting is lexicographic (string-based), assuming version format like "1.2.3" or timestamps.
func (fc *FilesystemCache) ListVersions(name string) ([]*PackageVersion, error) {
	packageDir := filepath.Join(fc.storagePath, name)

	// Check if package directory exists
	entries, err := os.ReadDir(packageDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*PackageVersion{}, nil
		}
		return nil, err
	}

	var versions []*PackageVersion
	suffix := ".pkg"

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()
		if !strings.HasSuffix(fileName, suffix) {
			continue
		}

		// Extract version from filename: "name-version.pkg" -> "version"
		prefix := fmt.Sprintf("%s-", name)
		if !strings.HasPrefix(fileName, prefix) {
			continue
		}

		versionPart := fileName[len(prefix) : len(fileName)-len(suffix)]
		path := filepath.Join(packageDir, fileName)

		versions = append(versions, &PackageVersion{
			Name:    name,
			Version: versionPart,
			Path:    path,
		})
	}

	// Sort newest-first (reverse lexicographic order)
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Version > versions[j].Version
	})

	logger.Debug("listed package versions", "name", name, "count", len(versions))
	return versions, nil
}

// RetainMostRecent retains only the `keep` most recent versions for name.
func (fc *FilesystemCache) RetainMostRecent(name string, keep int) error {
	if keep < 0 {
		return fmt.Errorf("keep must be non-negative")
	}

	versions, err := fc.ListVersions(name)
	if err != nil {
		return err
	}

	if len(versions) <= keep {
		logger.Debug("no cleanup needed", "name", name, "count", len(versions), "keep", keep)
		return nil
	}

	// Versions are sorted newest-first, so we delete from 'keep' onward
	for i := keep; i < len(versions); i++ {
		if err := fc.Remove(name, versions[i].Version); err != nil {
			logger.Error("failed to remove old version", "name", name, "version", versions[i].Version, "error", err)
			// Continue trying to remove other versions
		}
	}

	logger.Info("retained most recent versions", "name", name, "kept", keep, "total_before", len(versions))
	return nil
}

// Remove deletes a specific package version from the cache.
func (fc *FilesystemCache) Remove(name, version string) error {
	packageDir := filepath.Join(fc.storagePath, name)
	pkgFile := filepath.Join(packageDir, fmt.Sprintf("%s-%s.pkg", name, version))

	if err := os.Remove(pkgFile); err != nil {
		if os.IsNotExist(err) {
			logger.Debug("package file not found for removal", "path", pkgFile)
			return nil
		}
		return fmt.Errorf("failed to remove package file: %w", err)
	}

	logger.Debug("package removed from cache", "name", name, "version", version)
	return nil
}

// GetStoragePath returns the storage path used by this cache.
func (fc *FilesystemCache) GetStoragePath() string {
	return fc.storagePath
}

// GetPackageDir returns the directory path for a specific package.
func (fc *FilesystemCache) GetPackageDir(name string) string {
	return filepath.Join(fc.storagePath, name)
}

// GetPackagePath returns the expected storage path for a package file.
func (fc *FilesystemCache) GetPackagePath(name, version string) string {
	return filepath.Join(fc.storagePath, name, fmt.Sprintf("%s-%s.pkg", name, version))
}

// Cleanup removes all empty package directories.
func (fc *FilesystemCache) Cleanup() error {
	entries, err := os.ReadDir(fc.storagePath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		packageDir := filepath.Join(fc.storagePath, entry.Name())
		subEntries, err := os.ReadDir(packageDir)
		if err != nil {
			continue
		}

		// Check if directory is empty (no package files)
		isEmpty := true
		for _, subEntry := range subEntries {
			if !subEntry.IsDir() && strings.HasSuffix(subEntry.Name(), ".pkg") {
				isEmpty = false
				break
			}
		}

		if isEmpty {
			if err := os.RemoveAll(packageDir); err != nil {
				logger.Debug("failed to cleanup empty directory", "path", packageDir, "error", err)
			} else {
				logger.Debug("cleaned up empty package directory", "path", packageDir)
			}
		}
	}

	return nil
}
