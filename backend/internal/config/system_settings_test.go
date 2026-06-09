package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func withTempConfigDir(t *testing.T) {
	t.Helper()

	oldConfig := sysConfig
	sysConfig = nil
	t.Cleanup(func() {
		sysConfig = oldConfig
	})

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir(temp) error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	if err := os.MkdirAll("data", 0755); err != nil {
		t.Fatalf("MkdirAll(data) error: %v", err)
	}
}

func TestConfigFileReadsRuntimeSystemSettings(t *testing.T) {
	withTempConfigDir(t)

	configPath := filepath.Join("data", "config.ini")
	if err := os.WriteFile(configPath, []byte(`[opensync]
expires=9
task_save=30
task_timeout=12
copy_concurrency=13
scan_concurrency=17
realtime_finished_items=3500
max_retries=4
`), 0644); err != nil {
		t.Fatalf("WriteFile(config.ini) error: %v", err)
	}

	cfg := GetConfig()
	if cfg.Server.Expires != 9 {
		t.Fatalf("Expires = %d, want 9", cfg.Server.Expires)
	}
	if cfg.Server.TaskSave != 30 {
		t.Fatalf("TaskSave = %d, want 30", cfg.Server.TaskSave)
	}
	if cfg.Server.Timeout != 12 {
		t.Fatalf("Timeout = %d, want 12", cfg.Server.Timeout)
	}
	if cfg.Server.CopyConcurrency != 13 {
		t.Fatalf("CopyConcurrency = %d, want 13", cfg.Server.CopyConcurrency)
	}
	if cfg.Server.ScanConcurrency != 17 {
		t.Fatalf("ScanConcurrency = %d, want 17", cfg.Server.ScanConcurrency)
	}
	if cfg.Server.RealtimeFinishedItems != 3500 {
		t.Fatalf("RealtimeFinishedItems = %d, want 3500", cfg.Server.RealtimeFinishedItems)
	}
	if cfg.Server.MaxRetries != 4 {
		t.Fatalf("MaxRetries = %d, want 4", cfg.Server.MaxRetries)
	}
}

func TestUpdateSystemSettingsPersistsAndUpdatesMemory(t *testing.T) {
	withTempConfigDir(t)
	_ = GetConfig()

	settings := SystemSettings{
		Expires:               6,
		TaskTimeout:           48,
		TaskSave:              15,
		CopyConcurrency:       11,
		ScanConcurrency:       19,
		RealtimeFinishedItems: 4500,
		MaxRetries:            3,
	}
	if err := UpdateSystemSettings(settings); err != nil {
		t.Fatalf("UpdateSystemSettings() error: %v", err)
	}

	got := GetSystemSettings()
	if got != settings {
		t.Fatalf("GetSystemSettings() = %#v, want %#v", got, settings)
	}

	content, err := os.ReadFile(filepath.Join("data", "config.ini"))
	if err != nil {
		t.Fatalf("ReadFile(config.ini) error: %v", err)
	}
	text := string(content)
	for _, want := range []string{
		"expires=6",
		"task_timeout=48",
		"task_save=15",
		"copy_concurrency=11",
		"scan_concurrency=19",
		"realtime_finished_items=4500",
		"max_retries=3",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("config.ini missing %q in:\n%s", want, text)
		}
	}
}

func TestUpdateSystemSettingsRejectsScanConcurrencyAboveTwenty(t *testing.T) {
	withTempConfigDir(t)
	_ = GetConfig()

	before := GetSystemSettings()
	err := UpdateSystemSettings(SystemSettings{
		Expires:               before.Expires,
		TaskTimeout:           before.TaskTimeout,
		TaskSave:              before.TaskSave,
		CopyConcurrency:       before.CopyConcurrency,
		ScanConcurrency:       21,
		RealtimeFinishedItems: before.RealtimeFinishedItems,
		MaxRetries:            before.MaxRetries,
	})
	if err == nil {
		t.Fatalf("UpdateSystemSettings() error = nil, want validation error")
	}
	after := GetSystemSettings()
	if after != before {
		t.Fatalf("settings changed after invalid update: got %#v, want %#v", after, before)
	}
}

func TestUpdateSystemSettingsRejectsMaxRetriesAboveTen(t *testing.T) {
	withTempConfigDir(t)
	_ = GetConfig()

	before := GetSystemSettings()
	err := UpdateSystemSettings(SystemSettings{
		Expires:               before.Expires,
		TaskTimeout:           before.TaskTimeout,
		TaskSave:              before.TaskSave,
		CopyConcurrency:       before.CopyConcurrency,
		ScanConcurrency:       before.ScanConcurrency,
		RealtimeFinishedItems: before.RealtimeFinishedItems,
		MaxRetries:            11,
	})
	if err == nil {
		t.Fatalf("UpdateSystemSettings() error = nil, want validation error")
	}
	after := GetSystemSettings()
	if after != before {
		t.Fatalf("settings changed after invalid update: got %#v, want %#v", after, before)
	}
}

func TestSystemSettingsCanBeReadWhileUpdated(t *testing.T) {
	withTempConfigDir(t)
	_ = GetConfig()

	start := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 200; i++ {
			settings := SystemSettings{
				Expires:               6 + i%3,
				TaskTimeout:           24 + i%5,
				TaskSave:              15 + i%7,
				CopyConcurrency:       3 + i%5,
				ScanConcurrency:       2 + i%4,
				RealtimeFinishedItems: 1000 + i%100,
				MaxRetries:            i % 4,
			}
			if err := UpdateSystemSettings(settings); err != nil {
				t.Errorf("UpdateSystemSettings() error: %v", err)
				return
			}
		}
	}()

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 1000; j++ {
				_ = GetSystemSettings()
			}
		}()
	}

	close(start)
	wg.Wait()
}
