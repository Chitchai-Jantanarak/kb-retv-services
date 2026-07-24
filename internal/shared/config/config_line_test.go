package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromReadsLineChannelSecret(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	envPath := filepath.Join(dir, ".env")

	if err := os.WriteFile(configPath, []byte("app:\n  env: development\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	if err := os.WriteFile(envPath, []byte("LINE_CHANNEL_SECRET=line-secret-env\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(env) error = %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if cfg.Line.ChannelSecret != "line-secret-env" {
		t.Fatalf("Line.ChannelSecret = %q, want line-secret-env", cfg.Line.ChannelSecret)
	}
}

func TestLoadDefaultsLineChannelSecretEmpty(t *testing.T) {
	cfg, err := LoadFrom("")
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if cfg.Line.ChannelSecret != "" {
		t.Fatalf("Line.ChannelSecret = %q, want empty default", cfg.Line.ChannelSecret)
	}
}
