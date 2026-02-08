package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// MirrorConfig describes a mirror entry in the YAML config.
type MirrorConfig struct {
	URL    string  `yaml:"url" validate:"required,url"`
	Weight float64 `yaml:"weight" validate:"min=0.1"`
}

// Config is the top-level configuration for Trinity-cache.
type Config struct {
	Concurrency int            `yaml:"concurrency" validate:"min=1,max=10000"`
	StoragePath string         `yaml:"storage_path" validate:"required,dirpath"`
	Mirrors     []MirrorConfig `yaml:"mirrors" validate:"required,min=1,dive"`
}

// Validate checks if the configuration is valid.
// It verifies required fields, ranges, and constraints.
func (c *Config) Validate() error {
	// Validate concurrency
	if c.Concurrency <= 0 {
		return fmt.Errorf("concurrency must be positive, got %d", c.Concurrency)
	}
	if c.Concurrency > 10000 {
		return fmt.Errorf("concurrency exceeds maximum limit of 10000, got %d", c.Concurrency)
	}

	// Validate storage path
	if c.StoragePath == "" {
		return fmt.Errorf("storage_path is required")
	}

	// Validate mirrors
	if len(c.Mirrors) == 0 {
		return fmt.Errorf("at least one mirror must be configured")
	}

	for i, m := range c.Mirrors {
		if m.URL == "" {
			return fmt.Errorf("mirror %d: url is required", i)
		}
		if m.Weight <= 0 {
			return fmt.Errorf("mirror %d: weight must be positive, got %v", i, m.Weight)
		}
	}

	return nil
}

// Default returns a sensible default configuration.
func Default() *Config {
	return &Config{
		Concurrency: 8,
		StoragePath: "/var/lib/trinity-cache",
		Mirrors:     []MirrorConfig{},
	}
}

// Load reads the YAML config at path. If path is empty, returns defaults.
// It applies reasonable defaults and validates required fields.
func Load(path string) (*Config, error) {
	if path == "" {
		return Default(), nil
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	// apply defaults
	if c.Concurrency == 0 {
		c.Concurrency = 8
	}
	if c.StoragePath == "" {
		c.StoragePath = "/var/lib/trinity-cache"
	}

	// Set default weight for mirrors that don't have one
	for i := range c.Mirrors {
		if c.Mirrors[i].Weight == 0 {
			c.Mirrors[i].Weight = 1.0
		}
	}

	// validate
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return &c, nil
}
