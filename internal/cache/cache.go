package cache

// PackageVersion is a small descriptor for a stored package version.
type PackageVersion struct {
    Name    string
    Version string
    Path    string
}

// CacheManager manages a local package cache and enforces retention rules.
type CacheManager interface {
    // Has returns true if the given package name/version exists in cache.
    Has(name, version string) (bool, error)

    // GetLatest returns the most recent PackageVersion for name.
    GetLatest(name string) (*PackageVersion, error)

    // Add inserts a new package version into the cache.
    Add(p *PackageVersion) error

    // ListVersions lists versions known for a package name, ordered newest-first.
    ListVersions(name string) ([]*PackageVersion, error)

    // RetainMostRecent retains only the `keep` most recent versions for name.
    RetainMostRecent(name string, keep int) error

    // Remove deletes a specific package version from the cache.
    Remove(name, version string) error
}
