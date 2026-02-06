package versiontracker

// Tracker tracks available versions for packages and exposes queries.
type Tracker interface {
    // LatestVersion returns the latest known version for a package name.
    LatestVersion(name string) (string, error)

    // Update records a new version for the package name.
    Update(name, version string) error

    // ListVersions returns all known versions for a package, newest-first.
    ListVersions(name string) ([]string, error)
}
