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
	if cfg.Server.MaxRetries != 4 {
		t.Fatalf("MaxRetries = %d, want 4", cfg.Server.MaxRetries)
	}
}

func TestUpdateSystemSettingsPersistsAndUpdatesMemory(t *testing.T) {
	withTempConfigDir(t)
	_ = GetConfig()

	settings := SystemSettings{
		Expires:         6,
		TaskTimeout:     48,
		TaskSave:        15,
		CopyConcurrency: 11,
		ScanConcurrency: 19,
		MaxRetries:      3,
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
		"max_retries=3",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("config.ini missing %q in:\n%s", want, text)
		}
	}
}

func TestWriteConfigFileInsertsMissingKeysWithoutDuplicatingOpensyncSection(t *testing.T) {
	withTempConfigDir(t)

	configPath := filepath.Join("data", "config.ini")
	initial := `# top comment
[opensync]
# operator note
expires=5
custom_flag=enabled
`
	if err := os.WriteFile(configPath, []byte(initial), 0644); err != nil {
		t.Fatalf("WriteFile(config.ini) error: %v", err)
	}

	_ = GetConfig()
	settings := SystemSettings{
		Expires:         14,
		TaskTimeout:     24,
		TaskSave:        60,
		CopyConcurrency: 8,
		ScanConcurrency: 10,
		MaxRetries:      5,
	}
	if err := UpdateSystemSettings(settings); err != nil {
		t.Fatalf("UpdateSystemSettings() error: %v", err)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.ini) error: %v", err)
	}
	text := string(raw)
	if got := strings.Count(text, "[opensync]"); got != 1 {
		t.Fatalf("[opensync] section count = %d, want 1 in:\n%s", got, text)
	}
	for _, want := range []string{"# top comment", "# operator note", "custom_flag=enabled", "expires=14", "max_retries=5"} {
		if !strings.Contains(text, want) {
			t.Fatalf("config.ini missing %q in:\n%s", want, text)
		}
	}

	ini := readINI(configPath)
	if ini["opensync"]["expires"] != "14" || ini["opensync"]["custom_flag"] != "enabled" {
		t.Fatalf("reloaded INI = %#v, want expires=14 and custom_flag=enabled", ini["opensync"])
	}
}

func TestWriteConfigFileDeduplicatesExistingDuplicateOpensyncSections(t *testing.T) {
	withTempConfigDir(t)

	configPath := filepath.Join("data", "config.ini")
	initial := `[opensync]
expires=5
custom1=alpha

[other]
foo=bar

[opensync]
expires=99
custom2=beta
`
	if err := os.WriteFile(configPath, []byte(initial), 0644); err != nil {
		t.Fatalf("WriteFile(config.ini) error: %v", err)
	}

	_ = GetConfig()
	settings := SystemSettings{
		Expires:         12,
		TaskTimeout:     36,
		TaskSave:        20,
		CopyConcurrency: 6,
		ScanConcurrency: 9,
		MaxRetries:      2,
	}
	if err := UpdateSystemSettings(settings); err != nil {
		t.Fatalf("UpdateSystemSettings() error: %v", err)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.ini) error: %v", err)
	}
	text := string(raw)
	if got := strings.Count(text, "[opensync]"); got != 1 {
		t.Fatalf("[opensync] section count = %d, want 1 in:\n%s", got, text)
	}
	if strings.Contains(text, "expires=99") || strings.Contains(text, "expires=5") {
		t.Fatalf("stale expires values remained in:\n%s", text)
	}

	// Simulate restart: reload config from disk and confirm values did not revert.
	sysConfig = nil
	reloaded := GetConfig()
	if reloaded.Server.Expires != 12 {
		t.Fatalf("reloaded Expires = %d, want 12", reloaded.Server.Expires)
	}
	ini := readINI(configPath)
	if ini["other"]["foo"] != "bar" {
		t.Fatalf("other section lost: %#v", ini)
	}
	if ini["opensync"]["custom1"] != "alpha" || ini["opensync"]["custom2"] != "beta" {
		t.Fatalf("custom keys not preserved during merge: %#v", ini["opensync"])
	}
}

func TestUpdateSystemSettingsRejectsScanConcurrencyAboveTwenty(t *testing.T) {
	withTempConfigDir(t)
	_ = GetConfig()

	before := GetSystemSettings()
	err := UpdateSystemSettings(SystemSettings{
		Expires:         before.Expires,
		TaskTimeout:     before.TaskTimeout,
		TaskSave:        before.TaskSave,
		CopyConcurrency: before.CopyConcurrency,
		ScanConcurrency: 21,
		MaxRetries:      before.MaxRetries,
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
		Expires:         before.Expires,
		TaskTimeout:     before.TaskTimeout,
		TaskSave:        before.TaskSave,
		CopyConcurrency: before.CopyConcurrency,
		ScanConcurrency: before.ScanConcurrency,
		MaxRetries:      11,
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
				Expires:         6 + i%3,
				TaskTimeout:     24 + i%5,
				TaskSave:        15 + i%7,
				CopyConcurrency: 3 + i%5,
				ScanConcurrency: 2 + i%4,
				MaxRetries:      i % 4,
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
