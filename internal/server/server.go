package server

import "github.com/tommahs/trinity-cache/internal/cache"

// Server provides the package-serving surface for clients.
// Implementations should return errors explicitly and log failures.
type Server interface {
	// Serve starts the server listening on the given port.
	// Returns an error if the server fails to start.
	Serve(port int) error

	// FetchAndServe ensures the requested package/version is available,
	// fetching it if necessary, and makes it available to be served.
	// Return errors for failures; log when fetches occur.
	FetchAndServe(name, version string) error

	// SetCache attaches a CacheManager implementation to the server.
	SetCache(c cache.CacheManager)
}
