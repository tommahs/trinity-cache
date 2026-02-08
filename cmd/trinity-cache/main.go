package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tommahs/trinity-cache/internal/config"
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

	var cfg *config.Config
	var err error
	if *configPath != "" {
		cfg, err = config.Load(*configPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "config error:", err)
			os.Exit(1)
		}
		fmt.Println("Using config:", *configPath)
	} else {
		cfg = config.Default()
		fmt.Println("No config provided; using defaults")
	}
	if len(cfg.Mirrors) == 0 {
		fmt.Fprintln(os.Stderr, "config error: no mirrors configured")
		os.Exit(1)
	}
	fmt.Printf("Trinity-cache starting — version %s (concurrency=%d, storage=%s, mirrors=%d)\n",
		version.Version, cfg.Concurrency, cfg.StoragePath, len(cfg.Mirrors))
}
