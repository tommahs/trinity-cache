package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// MirrorConfig describes a mirror entry in the YAML config.
type MirrorConfig struct {
	URL     string  `yaml:"url" validate:"required,url"`
	Weight  float64 `yaml:"weight" validate:"min=0.1"`
	Timeout int     `yaml:"timeout" validate:"min=1"` // seconds
}

// ServerConfig contains HTTP server settings.
type ServerConfig struct {
	Port         string `yaml:"port" validate:"required"`           // e.g., ":8080"
	ReadTimeout  int    `yaml:"read_timeout" validate:"min=1"`      // seconds
	WriteTimeout int    `yaml:"write_timeout" validate:"min=1"`     // seconds
}

// RetentionConfig contains cache retention settings.
type RetentionConfig struct {
	KeepVersions        int     `yaml:"keep_versions" validate:"min=1"`       // minimum 1
	EnforcementInterval float64 `yaml:"enforcement_interval" validate:"min=0.1"` // hours
}

// MirrorRecoveryConfig contains mirror weight recovery settings.
type MirrorRecoveryConfig struct {
	Interval int     `yaml:"interval" validate:"min=1"`       // minutes
	Rate     float64 `yaml:"rate" validate:"min=0.01,max=1.0"` // 0.0-1.0
}

// DownloadConfig contains download settings.
type DownloadConfig struct {
	MaxRetries int    `yaml:"max_retries" validate:"min=1"`  // minimum 1 retry
	Timeout    int    `yaml:"timeout" validate:"min=1"`      // seconds
	TempDir    string `yaml:"temp_dir"`                       // empty = system temp
}

// Config is the top-level configuration for Trinity-cache.
type Config struct {
	Concurrency    int                    `yaml:"concurrency" validate:"min=1,max=10000"`
	StoragePath    string                 `yaml:"storage_path" validate:"required,dirpath"`
	LogLevel       string                 `yaml:"log_level" validate:"omitempty,oneof=debug info warn error"`
	Mirrors        []MirrorConfig         `yaml:"mirrors" validate:"required,min=1,dive"`
	Server         ServerConfig           `yaml:"server"`
	Retention      RetentionConfig        `yaml:"retention"`
	MirrorRecovery MirrorRecoveryConfig   `yaml:"mirror_recovery"`
	Downloads      DownloadConfig         `yaml:"downloads"`
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
		if m.Timeout < 1 && m.Timeout != 0 {
			return fmt.Errorf("mirror %d: timeout must be positive, got %d", i, m.Timeout)
		}
	}

	// Validate log level
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if c.LogLevel != "" && !validLevels[c.LogLevel] {
		return fmt.Errorf("invalid log level: %s (must be one of: debug, info, warn, error)", c.LogLevel)
	}

	// Validate server config
	if c.Server.ReadTimeout < 1 && c.Server.ReadTimeout != 0 {
		return fmt.Errorf("server.read_timeout must be positive, got %d", c.Server.ReadTimeout)
	}
	if c.Server.WriteTimeout < 1 && c.Server.WriteTimeout != 0 {
		return fmt.Errorf("server.write_timeout must be positive, got %d", c.Server.WriteTimeout)
	}

	// Validate retention config
	if c.Retention.KeepVersions < 1 && c.Retention.KeepVersions != 0 {
		return fmt.Errorf("retention.keep_versions must be at least 1, got %d", c.Retention.KeepVersions)
	}
	if c.Retention.EnforcementInterval < 0.1 && c.Retention.EnforcementInterval != 0 {
		return fmt.Errorf("retention.enforcement_interval must be at least 0.1 hours, got %v", c.Retention.EnforcementInterval)
	}

	// Validate mirror recovery config
	if c.MirrorRecovery.Interval < 1 && c.MirrorRecovery.Interval != 0 {
		return fmt.Errorf("mirror_recovery.interval must be positive, got %d", c.MirrorRecovery.Interval)
	}
	if c.MirrorRecovery.Rate < 0.01 && c.MirrorRecovery.Rate != 0 {
		return fmt.Errorf("mirror_recovery.rate must be between 0.01 and 1.0, got %v", c.MirrorRecovery.Rate)
	}
	if c.MirrorRecovery.Rate > 1.0 {
		return fmt.Errorf("mirror_recovery.rate must be between 0.01 and 1.0, got %v", c.MirrorRecovery.Rate)
	}

	// Validate download config
	if c.Downloads.MaxRetries < 1 && c.Downloads.MaxRetries != 0 {
		return fmt.Errorf("downloads.max_retries must be at least 1, got %d", c.Downloads.MaxRetries)
	}
	if c.Downloads.Timeout < 1 && c.Downloads.Timeout != 0 {
		return fmt.Errorf("downloads.timeout must be positive, got %d", c.Downloads.Timeout)
	}

	return nil
}

// Default returns a sensible default configuration.
func Default() *Config {
	return &Config{
		Concurrency: 8,
		StoragePath: "/var/lib/trinity-cache",
		LogLevel:    "info",
		Mirrors:     []MirrorConfig{},
		Server: ServerConfig{
			Port:         ":8080",
			ReadTimeout:  30,
			WriteTimeout: 30,
		},
		Retention: RetentionConfig{
			KeepVersions:        2,
			EnforcementInterval: 1.0, // 1 hour
		},
		MirrorRecovery: MirrorRecoveryConfig{
			Interval: 5,   // 5 minutes
			Rate:     0.05, // 5% recovery per interval
		},
		Downloads: DownloadConfig{
			MaxRetries: 3,
			Timeout:    30, // seconds
			TempDir:    "", // use system temp
		},
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

	// Apply defaults for top-level fields
	if c.Concurrency == 0 {
		c.Concurrency = 8
	}
	if c.StoragePath == "" {
		c.StoragePath = "/var/lib/trinity-cache"
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}

	// Apply defaults for server config
	if c.Server.Port == "" {
		c.Server.Port = ":8080"
	}
	if c.Server.ReadTimeout == 0 {
		c.Server.ReadTimeout = 30
	}
	if c.Server.WriteTimeout == 0 {
		c.Server.WriteTimeout = 30
	}

	// Apply defaults for retention config
	if c.Retention.KeepVersions == 0 {
		c.Retention.KeepVersions = 2
	}
	if c.Retention.EnforcementInterval == 0 {
		c.Retention.EnforcementInterval = 1.0 // 1 hour
	}

	// Apply defaults for mirror recovery config
	if c.MirrorRecovery.Interval == 0 {
		c.MirrorRecovery.Interval = 5 // 5 minutes
	}
	if c.MirrorRecovery.Rate == 0 {
		c.MirrorRecovery.Rate = 0.05 // 5% recovery per interval
	}

	// Apply defaults for downloads config
	if c.Downloads.MaxRetries == 0 {
		c.Downloads.MaxRetries = 3
	}
	if c.Downloads.Timeout == 0 {
		c.Downloads.Timeout = 30 // seconds
	}
	// TempDir defaults to empty string (system temp)

	// Set default weight and timeout for mirrors
	for i := range c.Mirrors {
		if c.Mirrors[i].Weight == 0 {
			c.Mirrors[i].Weight = 1.0
		}
		if c.Mirrors[i].Timeout == 0 {
			c.Mirrors[i].Timeout = 30 // seconds
		}
	}

	// Validate
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return &c, nil
}
