package config
package config

import (
    "fmt"
    "os"

    "gopkg.in/yaml.v3"
)

// MirrorConfig describes a mirror entry in the YAML config.
type MirrorConfig struct {
    URL    string  `yaml:"url"`
    Weight float64 `yaml:"weight"`
}

// Config is the top-level configuration for Trinity-cache.
type Config struct {
    Concurrency int            `yaml:"concurrency"`
    StoragePath string         `yaml:"storage_path"`
    Mirrors     []MirrorConfig `yaml:"mirrors"`
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

    // validate
    if len(c.Mirrors) == 0 {
        return &c, fmt.Errorf("no mirrors configured")
    }
    for i := range c.Mirrors {
        if c.Mirrors[i].URL == "" {
            return &c, fmt.Errorf("mirror %d missing url", i)
        }
        if c.Mirrors[i].Weight == 0 {
            c.Mirrors[i].Weight = 1.0
        }
    }

    return &c, nil
}
