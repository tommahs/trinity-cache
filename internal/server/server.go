package server

import "github.com/tommahs/trinity-cache/internal/cache"

// Server provides the package-serving surface for clients.
type Server interface {
    // Serve starts the server listening on the given port.
    Serve(port int) error

    // FetchAndServe ensures the requested package/version is available,
    // fetching it if necessary, and makes it available to be served.
    FetchAndServe(name, version string) error

    // SetCache attaches a CacheManager implementation to the server.
    SetCache(c cache.CacheManager)
}
