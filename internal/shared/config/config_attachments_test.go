package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromReadsLaravelAttachmentFetchFields(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	envPath := filepath.Join(dir, ".env")

	if err := os.WriteFile(configPath, []byte("app:\n  env: development\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	if err := os.WriteFile(envPath, []byte("LARAVEL_BASE_URL=http://app.internal\nLARAVEL_HOST_HEADER=app.local\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(env) error = %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if cfg.Laravel.BaseURL != "http://app.internal" {
		t.Fatalf("Laravel.BaseURL = %q, want http://app.internal", cfg.Laravel.BaseURL)
	}
	if cfg.Laravel.HostHeader != "app.local" {
		t.Fatalf("Laravel.HostHeader = %q, want app.local", cfg.Laravel.HostHeader)
	}
}

func TestLoadDefaultsLaravelHostHeaderEmpty(t *testing.T) {
	cfg, err := LoadFrom("")
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if cfg.Laravel.HostHeader != "" {
		t.Fatalf("Laravel.HostHeader = %q, want empty default", cfg.Laravel.HostHeader)
	}
}
