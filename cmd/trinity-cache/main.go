// Package main implements Trinity-cache.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/tommahs/trinity-cache/internal/cache"
	"github.com/tommahs/trinity-cache/internal/config"
	"github.com/tommahs/trinity-cache/internal/downloader"
	"github.com/tommahs/trinity-cache/internal/logger"
	"github.com/tommahs/trinity-cache/internal/mirror"
	"github.com/tommahs/trinity-cache/internal/server"
	"github.com/tommahs/trinity-cache/internal/version"
	"github.com/tommahs/trinity-cache/internal/versiontracker"
)

// Application holds all application components
type Application struct {
	cache        cache.CacheManager
	selector     mirror.Selector
	downloader   downloader.Downloader
	workerPool   *downloader.WorkerPool
	fetchManager *downloader.FetchManager
	httpServer   server.Server
	retention    *cache.RetentionManager
	shutdown     chan os.Signal
	shutdownDone chan struct{}
	mu           sync.Mutex
}

func main() {
	configPath := flag.String("config", "", "Path to YAML config file")
	showVersion := flag.Bool("version", false, "Show version")
	serverPort := flag.String("port", "", "HTTP server port (overrides config)")
	flag.Parse()

	if *showVersion {
		fmt.Println("Trinity-cache version", version.Version)
		os.Exit(0)
	}

	// Load configuration
	var cfg *config.Config
	var err error
	if *configPath != "" {
		cfg, err = config.Load(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
			os.Exit(1)
		}
	} else {
		cfg = config.Default()
	}

	// Override port from CLI if provided
	if *serverPort != "" {
		cfg.Server.Port = ":" + *serverPort
	}
	// Ensure port has colon prefix
	if cfg.Server.Port != "" && cfg.Server.Port[0] != ':' {
		cfg.Server.Port = ":" + cfg.Server.Port
	}

	// Configure logger based on loaded config
	logger.SetLevel(logger.ParseLevel(cfg.LogLevel))

	// Validate mirrors are configured
	if len(cfg.Mirrors) == 0 {
		logger.Error("no mirrors configured")
		os.Exit(1)
	}

	// Log startup information
	logger.Info("Trinity-cache starting",
		"version", version.Version,
		"concurrency", cfg.Concurrency,
		"storage", cfg.StoragePath,
		"mirrors", len(cfg.Mirrors),
		"server", cfg.Server)

	// Initialize application
	app, err := NewApplication(cfg)
	if err != nil {
		logger.Error("failed to initialize application", "error", err)
		os.Exit(1)
	}

	// Run the application
	if err := app.Run(); err != nil {
		logger.Error("application error", "error", err)
		os.Exit(1)
	}
}

// CacheAdapter adapts cache.CacheManager to downloader.Cache interface
type CacheAdapter struct {
	manager cache.CacheManager
}

// NewCacheAdapter creates a new cache adapter
func NewCacheAdapter(manager cache.CacheManager) *CacheAdapter {
	return &CacheAdapter{manager: manager}
}

// GetPackagePath returns the path for a package
func (ca *CacheAdapter) GetPackagePath(name, version string) string {
	if fc, ok := ca.manager.(*cache.FilesystemCache); ok {
		return fc.GetPackagePath(name, version)
	}
	return ""
}

// Has checks if a package exists in the cache
func (ca *CacheAdapter) Has(name, version string) (bool, error) {
	return ca.manager.Has(name, version)
}

// Add adds a package to the cache
func (ca *CacheAdapter) Add(name, version, path string) error {
	pv := &cache.PackageVersion{
		Name:    name,
		Version: version,
		Path:    path,
	}
	return ca.manager.Add(pv)
}

// RetainMostRecent keeps only the most recent versions of a package
func (ca *CacheAdapter) RetainMostRecent(name string, keep int) error {
	return ca.manager.RetainMostRecent(name, keep)
}

// NewApplication initializes all application components
func NewApplication(cfg *config.Config) (*Application, error) {
	// Initialize cache
	cacheManager, err := cache.NewFilesystemCache(cfg.StoragePath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cache: %w", err)
	}

	// Initialize mirror selector
	selector := mirror.NewWeightedSelector()
	for _, m := range cfg.Mirrors {
		selector.Add(&mirror.Mirror{
			URL:             m.URL,
			BaseWeight:      m.Weight,
			EffectiveWeight: m.Weight,
		})
	}

	// Start mirror weight recovery using configured values
	recoveryInterval := time.Duration(cfg.MirrorRecovery.Interval) * time.Minute
	selector.StartRecovery(recoveryInterval, cfg.MirrorRecovery.Rate)

	// Initialize HTTP downloader with configured values
	// Create a cache adapter for the downloader
	cacheAdapter := NewCacheAdapter(cacheManager)
	tempDir := cfg.Downloads.TempDir
	httpDownloader, err := downloader.NewHTTPDownloader(
		selector,
		cacheAdapter,
		tempDir,
		cfg.Downloads.MaxRetries,
		cfg.Downloads.Timeout,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize downloader: %w", err)
	}

	// Initialize worker pool
	workerPool, err := downloader.NewWorkerPool(httpDownloader, selector, cfg.Concurrency)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize worker pool: %w", err)
	}

	// Start worker pool
	if err := workerPool.Start(); err != nil {
		return nil, fmt.Errorf("failed to start worker pool: %w", err)
	}

	// Initialize retention manager with configured values
	retention := cache.NewRetentionManager(cacheManager)
	if err := retention.SetRetentionCount(cfg.Retention.KeepVersions); err != nil {
		return nil, fmt.Errorf("failed to set retention to %v", cfg.Retention.KeepVersions)
	}
	enforcementInterval := time.Duration(cfg.Retention.EnforcementInterval * float64(time.Hour))
	if err := retention.StartPeriodicEnforcement(enforcementInterval); err != nil {
		return nil, fmt.Errorf("failed to set enforcementInterval to %v", enforcementInterval)
	}

	// Initialize version tracker
	versionTracker, err := versiontracker.NewInMemoryTracker(cacheManager)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize version tracker: %w", err)
	}

	// Initialize HTTP server with configured timeouts
	httpSrvr, err := server.NewHTTPServer(
		cacheManager,
		cfg.Server.Port,
		cfg.Server.ReadTimeout,
		cfg.Server.WriteTimeout,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize HTTP server: %w", err)
	}

	// Initialize fetch manager for on-demand downloads
	fetchManager, err := downloader.NewFetchManager(httpDownloader, selector, versionTracker)
	if err != nil {
		logger.Warn("failed to initialize fetch manager: %v", err)
		// Don't fail on fetch manager initialization - it's an optional feature
	}

	// Set fetch manager on HTTP server if available
	if fetchManager != nil && httpSrvr != nil {
		httpSrvr.SetFetchManager(fetchManager)
	}

	return &Application{
		cache:        cacheManager,
		selector:     selector,
		downloader:   httpDownloader,
		workerPool:   workerPool,
		fetchManager: fetchManager,
		httpServer:   httpSrvr,
		retention:    retention,
		shutdown:     make(chan os.Signal, 1),
		shutdownDone: make(chan struct{}),
	}, nil
}

// Run runs the application with signal handling for graceful shutdown
func (app *Application) Run() error {
	// Setup signal handling for graceful shutdown
	signal.Notify(app.shutdown, syscall.SIGINT, syscall.SIGTERM)

	// Start HTTP server
	if err := app.httpServer.Start(); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	logger.Info("application ready")

	// Wait for shutdown signal
	sig := <-app.shutdown
	logger.Info("received shutdown signal", "signal", sig.String())

	// Graceful shutdown with timeout
	return app.Shutdown()
}

// Shutdown gracefully shuts down all application components
func (app *Application) Shutdown() error {
	app.mu.Lock()
	defer app.mu.Unlock()

	logger.Info("initiating graceful shutdown")

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var errors []error

	// Stop worker pool
	logger.Debug("stopping download worker pool")

	if err := app.workerPool.Stop(); err != nil {
		logger.Error("application error", "error", err)
		os.Exit(1)
	}
	app.workerPool.WaitForCompletion()

	// Stop mirror recovery
	selector, _ := app.selector.(*mirror.WeightedSelector)
	if selector != nil {
		logger.Debug("stopping mirror recovery")
		selector.Stop()
	}

	// Stop retention manager
	logger.Debug("stopping retention manager")
	app.retention.Stop()

	// Shutdown HTTP server
	logger.Debug("shutting down HTTP server")
	if err := app.httpServer.Shutdown(ctx); err != nil {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		logger.Error("shutdown completed with errors")
		return errors[0]
	}

	logger.Info("graceful shutdown completed")
	close(app.shutdownDone)
	return nil
}

// WaitForShutdown waits for the application to finish shutting down
func (app *Application) WaitForShutdown() {
	<-app.shutdownDone
}
