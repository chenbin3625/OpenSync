package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigFileInvalidNumbersKeepDefaults(t *testing.T) {
	oldConfig := sysConfig
	sysConfig = nil
	defer func() {
		sysConfig = oldConfig
	}()

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir(temp) error: %v", err)
	}
	defer os.Chdir(oldWD)

	if err := os.MkdirAll("data", 0755); err != nil {
		t.Fatalf("MkdirAll(data) error: %v", err)
	}
	configPath := filepath.Join("data", "config.ini")
	if err := os.WriteFile(configPath, []byte(`[opensync]
port=not-a-number
expires=5
task_timeout=also-bad
`), 0644); err != nil {
		t.Fatalf("WriteFile(config.ini) error: %v", err)
	}

	cfg := GetConfig()
	if cfg.Server.Port != 8023 {
		t.Fatalf("Port = %d, want default 8023 for invalid config value", cfg.Server.Port)
	}
	if cfg.Server.Expires != 5 {
		t.Fatalf("Expires = %d, want valid config override 5", cfg.Server.Expires)
	}
	if cfg.Server.Timeout != 72 {
		t.Fatalf("Timeout = %d, want default 72 for invalid config value", cfg.Server.Timeout)
	}
}
