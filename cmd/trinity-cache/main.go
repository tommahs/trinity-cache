package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tommahs/trinity-cache/internal/version"
)

func main() {
	config := flag.String("config", "", "Path to YAML config file")
	showVersion := flag.Bool("version", false, "Show version")
	flag.Parse()

	if *showVersion {
		fmt.Println("Trinity-cache version", version.Version)
		os.Exit(0)
	}

	fmt.Println("Trinity-cache starting — version", version.Version)
	if *config != "" {
		fmt.Println("Using config:", *config)
	} else {
		fmt.Println("No config provided; using defaults")
	}
}
