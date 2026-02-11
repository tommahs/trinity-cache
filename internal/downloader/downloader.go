//Package downloader implements the interface for downloading packages. 
package downloader

import "github.com/tommahs/trinity-cache/internal/mirror"

// Result represents the outcome of a download operation.
type Result struct {
	Path     string
	Size     int64
	Checksum string // SHA256 checksum of the file
}

// Downloader downloads packages from a given mirror.
// Implementations should return errors explicitly. Info logging for
// successful downloads and error logging for failures is recommended.
type Downloader interface {
	// Download downloads the package at pkgPath from the provided mirror.
	// It returns a Result describing where the downloaded file is stored.
	// Return errors for failures; log significant events.
	Download(m *mirror.Mirror, pkgPath string) (*Result, error)
}
