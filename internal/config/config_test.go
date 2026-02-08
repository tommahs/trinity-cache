package config

import (
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		errMsg  string
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Concurrency: 8,
				StoragePath: "/var/lib/trinity-cache",
				Mirrors: []MirrorConfig{
					{URL: "https://mirror1.archlinux.org", Weight: 1.0},
				},
			},
			wantErr: false,
		},
		{
			name: "multiple valid mirrors",
			config: &Config{
				Concurrency: 4,
				StoragePath: "/cache",
				Mirrors: []MirrorConfig{
					{URL: "https://mirror1.archlinux.org", Weight: 1.0},
					{URL: "https://mirror2.archlinux.org", Weight: 2.5},
				},
			},
			wantErr: false,
		},
		{
			name: "zero concurrency",
			config: &Config{
				Concurrency: 0,
				StoragePath: "/var/lib/trinity-cache",
				Mirrors: []MirrorConfig{
					{URL: "https://mirror1.archlinux.org", Weight: 1.0},
				},
			},
			errMsg:  "concurrency must be positive",
			wantErr: true,
		},
		{
			name: "negative concurrency",
			config: &Config{
				Concurrency: -5,
				StoragePath: "/var/lib/trinity-cache",
				Mirrors: []MirrorConfig{
					{URL: "https://mirror1.archlinux.org", Weight: 1.0},
				},
			},
			errMsg:  "concurrency must be positive",
			wantErr: true,
		},
		{
			name: "concurrency exceeds maximum",
			config: &Config{
				Concurrency: 50000,
				StoragePath: "/var/lib/trinity-cache",
				Mirrors: []MirrorConfig{
					{URL: "https://mirror1.archlinux.org", Weight: 1.0},
				},
			},
			errMsg:  "concurrency exceeds maximum limit",
			wantErr: true,
		},
		{
			name: "empty storage path",
			config: &Config{
				Concurrency: 8,
				StoragePath: "",
				Mirrors: []MirrorConfig{
					{URL: "https://mirror1.archlinux.org", Weight: 1.0},
				},
			},
			errMsg:  "storage_path is required",
			wantErr: true,
		},
		{
			name: "no mirrors",
			config: &Config{
				Concurrency: 8,
				StoragePath: "/var/lib/trinity-cache",
				Mirrors:     []MirrorConfig{},
			},
			errMsg:  "at least one mirror must be configured",
			wantErr: true,
		},
		{
			name: "mirror with empty URL",
			config: &Config{
				Concurrency: 8,
				StoragePath: "/var/lib/trinity-cache",
				Mirrors: []MirrorConfig{
					{URL: "", Weight: 1.0},
				},
			},
			errMsg:  "mirror 0: url is required",
			wantErr: true,
		},
		{
			name: "mirror with zero weight",
			config: &Config{
				Concurrency: 8,
				StoragePath: "/var/lib/trinity-cache",
				Mirrors: []MirrorConfig{
					{URL: "https://mirror1.archlinux.org", Weight: 0},
				},
			},
			errMsg:  "mirror 0: weight must be positive",
			wantErr: true,
		},
		{
			name: "mirror with negative weight",
			config: &Config{
				Concurrency: 8,
				StoragePath: "/var/lib/trinity-cache",
				Mirrors: []MirrorConfig{
					{URL: "https://mirror1.archlinux.org", Weight: -1.5},
				},
			},
			errMsg:  "mirror 0: weight must be positive",
			wantErr: true,
		},
		{
			name: "multiple mirrors with one invalid URL",
			config: &Config{
				Concurrency: 8,
				StoragePath: "/var/lib/trinity-cache",
				Mirrors: []MirrorConfig{
					{URL: "https://mirror1.archlinux.org", Weight: 1.0},
					{URL: "", Weight: 1.0},
				},
			},
			errMsg:  "mirror 1: url is required",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if err.Error() == "" || !contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %v, want error containing %s", err, tt.errMsg)
				}
			}
		})
	}
}

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Concurrency != 8 {
		t.Errorf("Default concurrency = %d, want 8", cfg.Concurrency)
	}
	if cfg.StoragePath != "/var/lib/trinity-cache" {
		t.Errorf("Default storage path = %s, want /var/lib/trinity-cache", cfg.StoragePath)
	}
	if len(cfg.Mirrors) != 0 {
		t.Errorf("Default mirrors count = %d, want 0", len(cfg.Mirrors))
	}
}

func TestLoadWithDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Errorf("Load() error = %v", err)
		return
	}

	if cfg.Concurrency != 8 {
		t.Errorf("Load(\"\") concurrency = %d, want 8", cfg.Concurrency)
	}
	if cfg.StoragePath != "/var/lib/trinity-cache" {
		t.Errorf("Load(\"\") storage path = %s, want /var/lib/trinity-cache", cfg.StoragePath)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
