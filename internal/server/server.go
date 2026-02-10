package server

import (
	"context"
	"github.com/tommahs/trinity-cache/internal/cache"
)

// Server provides the package-serving surface for clients.
// Implementations should return errors explicitly and log failures.
type Server interface {
	// Start starts the server listening.
	// Returns an error if the server fails to start.
	Start() error

	// Shutdown gracefully shuts down the server with context timeout.
	// Returns an error if shutdown fails.
	Shutdown(ctx context.Context) error

	// IsRunning returns whether the server is currently running.
	IsRunning() bool

	// FetchAndServe ensures the requested package/version is available,
	// fetching it if necessary, and makes it available to be served.
	// Return errors for failures; log when fetches occur.
	FetchAndServe(name, version string) error

	// SetCache attaches a CacheManager implementation to the server.
	SetCache(c cache.CacheManager)
}
