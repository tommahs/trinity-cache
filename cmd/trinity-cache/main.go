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
	cache          cache.CacheManager
	selector       mirror.Selector
	downloader     downloader.Downloader
	workerPool     *downloader.WorkerPool
	fetchManager   *downloader.FetchManager
	httpServer     server.Server
	retention      *cache.RetentionManager
	shutdown       chan os.Signal
	shutdownDone   chan struct{}
	mu             sync.Mutex
}

func main() {
	configPath := flag.String("config", "", "Path to YAML config file")
	showVersion := flag.Bool("version", false, "Show version")
	serverPort := flag.String("port", ":8080", "HTTP server port")
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
		"port", *serverPort)

	// Initialize application
	app, err := NewApplication(cfg, *serverPort)
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

// NewApplication initializes all application components
func NewApplication(cfg *config.Config, serverPort string) (*Application, error) {
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

	// Start mirror weight recovery
	selector.(*mirror.WeightedSelector).StartRecovery(5*time.Minute, 0.05)

	// Initialize HTTP downloader
	httpDownloader, err := downloader.NewHTTPDownloader(selector, cacheManager, "/tmp")
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

	// Initialize retention manager
	retention := cache.NewRetentionManager(cacheManager)
	retention.SetRetentionCount(2)
	retention.StartPeriodicEnforcement(1 * time.Hour)

	// Initialize version tracker
	versionTracker, err := versiontracker.NewInMemoryTracker(cacheManager)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize version tracker: %w", err)
	}

	// Initialize HTTP server
	httpServer, err := server.NewHTTPServer(cacheManager, serverPort)
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
	if fetchManager != nil && httpServer != nil {
		if httpSrvr, ok := httpServer.(*server.HTTPServer); ok {
			httpSrvr.SetFetchManager(fetchManager)
		}
	}

	return &Application{
		cache:        cacheManager,
		selector:     selector,
		downloader:   httpDownloader,
		workerPool:   workerPool,
		fetchManager: fetchManager,
		httpServer:   httpServer,
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
	app.workerPool.Stop()
	app.workerPool.WaitForCompletion()

	// Stop mirror recovery
	if selector, ok := app.selector.(*mirror.WeightedSelector); ok {
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

