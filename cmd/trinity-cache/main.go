package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/tommahs/trinity-cache/internal/config"
	"github.com/tommahs/trinity-cache/internal/logger"
	"github.com/tommahs/trinity-cache/internal/version"
)

func main() {
	configPath := flag.String("config", "", "Path to YAML config file")
	showVersion := flag.Bool("version", false, "Show version")
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
	logger.Info("Trinity-cache started",
		"version", version.Version,
		"concurrency", cfg.Concurrency,
		"storage", cfg.StoragePath,
		"mirrors", len(cfg.Mirrors))
}
