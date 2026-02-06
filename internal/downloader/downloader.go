package downloader

import "github.com/tommahs/trinity-cache/internal/mirror"

// Result represents the outcome of a download operation.
type Result struct {
    Path string
    Size int64
}

// Downloader downloads packages from a given mirror.
type Downloader interface {
    // Download downloads the package at pkgPath from the provided mirror.
    // It returns a Result describing where the downloaded file is stored.
    Download(m *mirror.Mirror, pkgPath string) (*Result, error)
}
