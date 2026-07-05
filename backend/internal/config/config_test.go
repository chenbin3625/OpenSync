package config

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
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
	if cfg.Server.Timeout != 48 {
		t.Fatalf("Timeout = %d, want default 48 for invalid config value", cfg.Server.Timeout)
	}
}

func TestEnvironmentInvalidNumbersKeepDefaultsAndLog(t *testing.T) {
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
	t.Setenv("OPENSYNC_PORT", "not-a-number")
	t.Setenv("OPENSYNC_EXPIRES", "6")
	t.Setenv("OPENSYNC_TASK_TIMEOUT", "also-bad")

	var logBuf bytes.Buffer
	oldWriter := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(oldWriter)

	cfg := GetConfig()
	if cfg.Server.Port != 8023 {
		t.Fatalf("Port = %d, want default 8023 for invalid env value", cfg.Server.Port)
	}
	if cfg.Server.Expires != 6 {
		t.Fatalf("Expires = %d, want valid env override 6", cfg.Server.Expires)
	}
	if cfg.Server.Timeout != 48 {
		t.Fatalf("Timeout = %d, want default 48 for invalid env value", cfg.Server.Timeout)
	}
	logs := logBuf.String()
	if !strings.Contains(logs, "OPENSYNC_PORT") || !strings.Contains(logs, "OPENSYNC_TASK_TIMEOUT") {
		t.Fatalf("logs = %q, want invalid env keys to be logged", logs)
	}
}
