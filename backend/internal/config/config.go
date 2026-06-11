package config

import (
	"bufio"
	"fmt"
	"log"
	"opensync/internal/i18n"
	"opensync/pkg/crypto"
	"os"
	"strconv"
	"strings"
	"sync"
)

// ServerConfig holds server configuration
type ServerConfig struct {
	Port                  int
	Expires               int
	LogLevel              int
	ConsoleLevel          int
	LogSave               int
	TaskSave              int
	Timeout               int
	CopyConcurrency       int
	ScanConcurrency       int
	RealtimeFinishedItems int
	MaxRetries            int
	PasswdStr             string
}

// DBConfig holds database configuration
type DBConfig struct {
	DBName string
}

// Config holds all configuration
type Config struct {
	Server ServerConfig
	DB     DBConfig
}

var (
	sysConfig *Config
	configMu  sync.RWMutex
)

const (
	defaultPort                  = 8023
	defaultExpires               = 7
	defaultLogLevel              = 1
	defaultConsoleLevel          = 2
	defaultLogSave               = 7
	defaultTaskSave              = 30
	defaultTaskTimeout           = 48
	defaultCopyConcurrency       = 5
	defaultScanConcurrency       = 8
	defaultRealtimeFinishedItems = 1000
	defaultMaxRetries            = 0

	minExpires               = 1
	maxExpires               = 365
	minTaskSave              = 0
	maxTaskSave              = 3650
	minTaskTimeout           = 0
	maxTaskTimeout           = 8760
	minCopyConcurrency       = 1
	maxCopyConcurrency       = 100
	minScanConcurrency       = 1
	maxScanConcurrency       = 20
	minRealtimeFinishedItems = 100
	maxRealtimeFinishedItems = 50000
	minMaxRetries            = 0
	maxMaxRetries            = 10
)

// SystemSettings is the subset of backend settings exposed for runtime editing.
type SystemSettings struct {
	Expires               int `json:"expires"`
	TaskTimeout           int `json:"taskTimeout"`
	TaskSave              int `json:"taskSave"`
	CopyConcurrency       int `json:"copyConcurrency"`
	ScanConcurrency       int `json:"scanConcurrency"`
	RealtimeFinishedItems int `json:"realtimeFinishedItems"`
	MaxRetries            int `json:"maxRetries"`
}

// GetPasswordStr gets or generates the encryption secret key
func GetPasswordStr() string {
	return crypto.ReadOrSetFile("data/secret.key", crypto.GeneratePassword(256), false)
}

// GetConfig returns the global config (singleton)
func GetConfig() *Config {
	configMu.RLock()
	cfg := sysConfig
	configMu.RUnlock()
	if cfg != nil {
		return cfg
	}

	configMu.Lock()
	defer configMu.Unlock()
	if sysConfig != nil {
		return sysConfig
	}

	passwdStr := GetPasswordStr()
	dbname := "data/openSync.db"

	sCfg := ServerConfig{
		Port:                  defaultPort,
		Expires:               defaultExpires,
		LogLevel:              defaultLogLevel,
		ConsoleLevel:          defaultConsoleLevel,
		LogSave:               defaultLogSave,
		TaskSave:              defaultTaskSave,
		Timeout:               defaultTaskTimeout,
		CopyConcurrency:       defaultCopyConcurrency,
		ScanConcurrency:       defaultScanConcurrency,
		RealtimeFinishedItems: defaultRealtimeFinishedItems,
		MaxRetries:            defaultMaxRetries,
		PasswdStr:             passwdStr,
	}

	if _, err := os.Stat("data/config.ini"); err == nil {
		// Read config.ini
		iniMap := readINI("data/config.ini")
		if opensync, ok := iniMap["opensync"]; ok {
			if v, ok := opensync["port"]; ok {
				sCfg.Port = intConfigValue(v, sCfg.Port, "port")
			}
			if v, ok := opensync["expires"]; ok {
				sCfg.Expires = intConfigValue(v, sCfg.Expires, "expires")
			}
			if v, ok := opensync["log_level"]; ok {
				sCfg.LogLevel = intConfigValue(v, sCfg.LogLevel, "log_level")
			}
			if v, ok := opensync["console_level"]; ok {
				sCfg.ConsoleLevel = intConfigValue(v, sCfg.ConsoleLevel, "console_level")
			}
			if v, ok := opensync["log_save"]; ok {
				sCfg.LogSave = intConfigValue(v, sCfg.LogSave, "log_save")
			}
			if v, ok := opensync["task_save"]; ok {
				sCfg.TaskSave = intConfigValue(v, sCfg.TaskSave, "task_save")
			}
			if v, ok := opensync["task_timeout"]; ok {
				sCfg.Timeout = intConfigValue(v, sCfg.Timeout, "task_timeout")
			}
			if v, ok := opensync["copy_concurrency"]; ok {
				sCfg.CopyConcurrency = intConfigValue(v, sCfg.CopyConcurrency, "copy_concurrency")
			}
			if v, ok := opensync["scan_concurrency"]; ok {
				sCfg.ScanConcurrency = intConfigValue(v, sCfg.ScanConcurrency, "scan_concurrency")
			}
			if v, ok := opensync["realtime_finished_items"]; ok {
				sCfg.RealtimeFinishedItems = intConfigValue(v, sCfg.RealtimeFinishedItems, "realtime_finished_items")
			}
			if v, ok := opensync["max_retries"]; ok {
				sCfg.MaxRetries = intConfigValue(v, sCfg.MaxRetries, "max_retries")
			}
		}
	} else {
		// Read from environment variables
		sCfg.Port = envIntConfigValue("OPENSYNC_PORT", sCfg.Port)
		sCfg.Expires = envIntConfigValue("OPENSYNC_EXPIRES", sCfg.Expires)
		sCfg.LogLevel = envIntConfigValue("OPENSYNC_LOG_LEVEL", sCfg.LogLevel)
		sCfg.ConsoleLevel = envIntConfigValue("OPENSYNC_CONSOLE_LEVEL", sCfg.ConsoleLevel)
		sCfg.LogSave = envIntConfigValue("OPENSYNC_LOG_SAVE", sCfg.LogSave)
		sCfg.TaskSave = envIntConfigValue("OPENSYNC_TASK_SAVE", sCfg.TaskSave)
		sCfg.Timeout = envIntConfigValue("OPENSYNC_TASK_TIMEOUT", sCfg.Timeout)
		sCfg.CopyConcurrency = envIntConfigValue("OPENSYNC_COPY_CONCURRENCY", sCfg.CopyConcurrency)
		sCfg.ScanConcurrency = envIntConfigValue("OPENSYNC_SCAN_CONCURRENCY", sCfg.ScanConcurrency)
		sCfg.RealtimeFinishedItems = envIntConfigValue("OPENSYNC_REALTIME_FINISHED_ITEMS", sCfg.RealtimeFinishedItems)
		sCfg.MaxRetries = envIntConfigValue("OPENSYNC_MAX_RETRIES", sCfg.MaxRetries)
	}

	sysConfig = &Config{
		DB:     DBConfig{DBName: dbname},
		Server: sCfg,
	}
	return sysConfig
}

// SetConfigForTest swaps the process config for tests in other packages.
func SetConfigForTest(cfg *Config) {
	configMu.Lock()
	defer configMu.Unlock()
	sysConfig = cfg
}

// GetSystemSettings returns the runtime-editable settings.
func GetSystemSettings() SystemSettings {
	cfg := GetConfig()
	return SystemSettings{
		Expires:               cfg.Server.Expires,
		TaskTimeout:           cfg.Server.Timeout,
		TaskSave:              cfg.Server.TaskSave,
		CopyConcurrency:       cfg.Server.CopyConcurrency,
		ScanConcurrency:       cfg.Server.ScanConcurrency,
		RealtimeFinishedItems: cfg.Server.RealtimeFinishedItems,
		MaxRetries:            cfg.Server.MaxRetries,
	}
}

// UpdateSystemSettings validates, persists, and applies runtime-editable settings.
func UpdateSystemSettings(settings SystemSettings) error {
	if err := validateSystemSettings(settings); err != nil {
		return err
	}

	cfg := GetConfig()
	nextServer := cfg.Server
	nextServer.Expires = settings.Expires
	nextServer.Timeout = settings.TaskTimeout
	nextServer.TaskSave = settings.TaskSave
	nextServer.CopyConcurrency = settings.CopyConcurrency
	nextServer.ScanConcurrency = settings.ScanConcurrency
	nextServer.RealtimeFinishedItems = settings.RealtimeFinishedItems
	nextServer.MaxRetries = settings.MaxRetries

	if err := writeConfigFile(nextServer); err != nil {
		return err
	}
	configMu.Lock()
	sysConfig = &Config{
		DB:     cfg.DB,
		Server: nextServer,
	}
	configMu.Unlock()
	return nil
}

func validateSystemSettings(settings SystemSettings) error {
	checks := []struct {
		name     string
		value    int
		min, max int
	}{
		{i18n.G("settings_expires"), settings.Expires, minExpires, maxExpires},
		{i18n.G("settings_task_timeout"), settings.TaskTimeout, minTaskTimeout, maxTaskTimeout},
		{i18n.G("settings_task_save"), settings.TaskSave, minTaskSave, maxTaskSave},
		{i18n.G("settings_copy_concurrency"), settings.CopyConcurrency, minCopyConcurrency, maxCopyConcurrency},
		{i18n.G("settings_scan_concurrency"), settings.ScanConcurrency, minScanConcurrency, maxScanConcurrency},
		{i18n.G("settings_realtime_finished_items"), settings.RealtimeFinishedItems, minRealtimeFinishedItems, maxRealtimeFinishedItems},
		{i18n.G("settings_max_retries"), settings.MaxRetries, minMaxRetries, maxMaxRetries},
	}
	for _, item := range checks {
		if item.value < item.min || item.value > item.max {
			return fmt.Errorf(i18n.G("settings_range_error"), item.name, item.min, item.max)
		}
	}
	return nil
}

func envIntConfigValue(envName string, fallback int) int {
	value := os.Getenv(envName)
	if value == "" {
		return fallback
	}
	return intConfigValue(value, fallback, envName)
}

func writeConfigFile(sCfg ServerConfig) error {
	if err := os.MkdirAll("data", 0755); err != nil {
		return err
	}
	content := fmt.Sprintf(`[opensync]
port=%d
expires=%d
log_level=%d
console_level=%d
log_save=%d
task_save=%d
task_timeout=%d
copy_concurrency=%d
scan_concurrency=%d
realtime_finished_items=%d
max_retries=%d
`,
		sCfg.Port,
		sCfg.Expires,
		sCfg.LogLevel,
		sCfg.ConsoleLevel,
		sCfg.LogSave,
		sCfg.TaskSave,
		sCfg.Timeout,
		sCfg.CopyConcurrency,
		sCfg.ScanConcurrency,
		sCfg.RealtimeFinishedItems,
		sCfg.MaxRetries,
	)
	return os.WriteFile("data/config.ini", []byte(content), 0644)
}

func intConfigValue(value string, fallback int, key string) int {
	i, err := strconv.Atoi(value)
	if err != nil {
		log.Printf("配置项 %s=%q 不是有效整数，将使用默认值 %d", key, value, fallback)
		return fallback
	}
	return i
}

// readINI parses a simple INI file
func readINI(filename string) map[string]map[string]string {
	result := make(map[string]map[string]string)
	f, err := os.Open(filename)
	if err != nil {
		log.Printf("配置文件读取失败: %v", err)
		return result
	}
	defer f.Close()

	var section string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line[1 : len(line)-1]
			if _, ok := result[section]; !ok {
				result[section] = make(map[string]string)
			}
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 && section != "" {
			result[section][strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return result
}
