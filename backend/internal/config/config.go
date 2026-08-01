package config

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"opensync/internal/msg"
	"opensync/pkg/crypto"
	"os"
	"strconv"
	"strings"
	"sync"
)

// ServerConfig holds server configuration
type ServerConfig struct {
	Bind            string
	Port            int
	Expires         int
	LogLevel        int
	ConsoleLevel    int
	LogSave         int
	TaskSave        int
	Timeout         int
	CopyConcurrency int
	ScanConcurrency int
	MaxRetries      int
	PasswdStr       string
	// TrustedProxies lists CIDR ranges (or bare IPs) whose X-Forwarded-Proto
	// header may be honored when deciding whether to mark the auth cookie
	// Secure behind a TLS-terminating reverse proxy.
	TrustedProxies []string
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
	defaultBind         = "0.0.0.0"
	defaultPort         = 8023
	defaultExpires      = 7
	defaultLogLevel     = 1
	defaultConsoleLevel = 2
	defaultLogSave      = 7
	defaultTaskSave     = 30
	defaultTaskTimeout  = 48

	minExpires     = 1
	maxExpires     = 365
	minTaskSave    = 0
	maxTaskSave    = 3650
	minTaskTimeout = 0
	maxTaskTimeout = 8760
)

const (
	DefaultCopyConcurrency = 5
	DefaultScanConcurrency = 8
	DefaultMaxRetries      = 2

	MinCopyConcurrency = 1
	MaxCopyConcurrency = 100
	MinScanConcurrency = 1
	MaxScanConcurrency = 20
	MinMaxRetries      = 0
	MaxRetryAttempts   = 10
)

// SystemSettings is the subset of backend settings exposed for runtime editing.
type SystemSettings struct {
	Expires         int `json:"expires"`
	TaskTimeout     int `json:"taskTimeout"`
	TaskSave        int `json:"taskSave"`
	CopyConcurrency int `json:"copyConcurrency"`
	ScanConcurrency int `json:"scanConcurrency"`
	MaxRetries      int `json:"maxRetries"`
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
		Bind:            defaultBind,
		Port:            defaultPort,
		Expires:         defaultExpires,
		LogLevel:        defaultLogLevel,
		ConsoleLevel:    defaultConsoleLevel,
		LogSave:         defaultLogSave,
		TaskSave:        defaultTaskSave,
		Timeout:         defaultTaskTimeout,
		CopyConcurrency: DefaultCopyConcurrency,
		ScanConcurrency: DefaultScanConcurrency,
		MaxRetries:      DefaultMaxRetries,
		PasswdStr:       passwdStr,
	}

	if _, err := os.Stat("data/config.ini"); err == nil {
		// Read config.ini
		iniMap := readINI("data/config.ini")
		if opensync, ok := iniMap["opensync"]; ok {
			if v, ok := opensync["bind"]; ok {
				sCfg.Bind = stringConfigValue(v, sCfg.Bind)
			}
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
			if v, ok := opensync["max_retries"]; ok {
				sCfg.MaxRetries = intConfigValue(v, sCfg.MaxRetries, "max_retries")
			}
			if v, ok := opensync["trusted_proxies"]; ok {
				sCfg.TrustedProxies = parseTrustedProxies(v)
			}
		}
	} else {
		// Read from environment variables
		sCfg.Bind = envStringConfigValue("OPENSYNC_BIND", sCfg.Bind)
		sCfg.Port = envIntConfigValue("OPENSYNC_PORT", sCfg.Port)
		sCfg.Expires = envIntConfigValue("OPENSYNC_EXPIRES", sCfg.Expires)
		sCfg.LogLevel = envIntConfigValue("OPENSYNC_LOG_LEVEL", sCfg.LogLevel)
		sCfg.ConsoleLevel = envIntConfigValue("OPENSYNC_CONSOLE_LEVEL", sCfg.ConsoleLevel)
		sCfg.LogSave = envIntConfigValue("OPENSYNC_LOG_SAVE", sCfg.LogSave)
		sCfg.TaskSave = envIntConfigValue("OPENSYNC_TASK_SAVE", sCfg.TaskSave)
		sCfg.Timeout = envIntConfigValue("OPENSYNC_TASK_TIMEOUT", sCfg.Timeout)
		sCfg.CopyConcurrency = envIntConfigValue("OPENSYNC_COPY_CONCURRENCY", sCfg.CopyConcurrency)
		sCfg.ScanConcurrency = envIntConfigValue("OPENSYNC_SCAN_CONCURRENCY", sCfg.ScanConcurrency)
		sCfg.MaxRetries = envIntConfigValue("OPENSYNC_MAX_RETRIES", sCfg.MaxRetries)
		sCfg.TrustedProxies = parseTrustedProxies(os.Getenv("OPENSYNC_TRUSTED_PROXIES"))
	}

	sysConfig = &Config{
		DB:     DBConfig{DBName: dbname},
		Server: sCfg,
	}
	clampServerConfig(&sysConfig.Server)
	return sysConfig
}

// clampServerConfig enforces the same ranges as validateSystemSettings on
// values loaded from config.ini or environment variables. Manually edited
// values that fall outside the allowed range fall back to the default rather
// than silently taking effect (e.g. copy_concurrency=99999 spawning an
// unbounded number of goroutines, or task_timeout=-1 producing a negative
// timeout).
func clampServerConfig(sCfg *ServerConfig) {
	sCfg.Bind = stringConfigValue(sCfg.Bind, defaultBind)
	sCfg.Expires = clampInt(sCfg.Expires, minExpires, maxExpires, defaultExpires)
	sCfg.Timeout = clampInt(sCfg.Timeout, minTaskTimeout, maxTaskTimeout, defaultTaskTimeout)
	sCfg.TaskSave = clampInt(sCfg.TaskSave, minTaskSave, maxTaskSave, defaultTaskSave)
	sCfg.CopyConcurrency = clampInt(sCfg.CopyConcurrency, MinCopyConcurrency, MaxCopyConcurrency, DefaultCopyConcurrency)
	sCfg.ScanConcurrency = clampInt(sCfg.ScanConcurrency, MinScanConcurrency, MaxScanConcurrency, DefaultScanConcurrency)
	sCfg.MaxRetries = clampInt(sCfg.MaxRetries, MinMaxRetries, MaxRetryAttempts, DefaultMaxRetries)
}

func clampInt(value, min, max, fallback int) int {
	if value < min || value > max {
		return fallback
	}
	return value
}

// parseTrustedProxies splits a comma-separated list of CIDRs / bare IPs.
func parseTrustedProxies(value string) []string {
	var proxies []string
	for _, part := range strings.Split(value, ",") {
		if p := strings.TrimSpace(part); p != "" {
			proxies = append(proxies, p)
		}
	}
	return proxies
}

// IsTrustedProxy reports whether remoteAddr (host:port) is a loopback address
// or falls within a configured trusted-proxy CIDR / equals a configured bare IP.
// Used to decide whether X-Forwarded-Proto may be honored for the Secure cookie
// attribute when the app is deployed behind a TLS-terminating reverse proxy.
func (s *ServerConfig) IsTrustedProxy(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	for _, entry := range s.TrustedProxies {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			if _, network, parseErr := net.ParseCIDR(entry); parseErr == nil && network.Contains(ip) {
				return true
			}
		} else if entry == ip.String() {
			return true
		}
	}
	return false
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
		Expires:         cfg.Server.Expires,
		TaskTimeout:     cfg.Server.Timeout,
		TaskSave:        cfg.Server.TaskSave,
		CopyConcurrency: cfg.Server.CopyConcurrency,
		ScanConcurrency: cfg.Server.ScanConcurrency,
		MaxRetries:      cfg.Server.MaxRetries,
	}
}

// UpdateSystemSettings validates, persists, and applies runtime-editable settings.
func UpdateSystemSettings(settings SystemSettings) error {
	if err := validateSystemSettings(settings); err != nil {
		return err
	}

	GetConfig()
	configMu.Lock()
	defer configMu.Unlock()

	cfg := sysConfig
	nextServer := cfg.Server
	nextServer.Expires = settings.Expires
	nextServer.Timeout = settings.TaskTimeout
	nextServer.TaskSave = settings.TaskSave
	nextServer.CopyConcurrency = settings.CopyConcurrency
	nextServer.ScanConcurrency = settings.ScanConcurrency
	nextServer.MaxRetries = settings.MaxRetries

	if err := writeConfigFile(nextServer); err != nil {
		return err
	}
	sysConfig = &Config{
		DB:     cfg.DB,
		Server: nextServer,
	}
	return nil
}

func validateSystemSettings(settings SystemSettings) error {
	checks := []struct {
		name     string
		value    int
		min, max int
	}{
		{msg.SettingsExpires, settings.Expires, minExpires, maxExpires},
		{msg.SettingsTaskTimeout, settings.TaskTimeout, minTaskTimeout, maxTaskTimeout},
		{msg.SettingsTaskSave, settings.TaskSave, minTaskSave, maxTaskSave},
		{msg.SettingsCopyConcurrency, settings.CopyConcurrency, MinCopyConcurrency, MaxCopyConcurrency},
		{msg.SettingsScanConcurrency, settings.ScanConcurrency, MinScanConcurrency, MaxScanConcurrency},
		{msg.SettingsMaxRetries, settings.MaxRetries, MinMaxRetries, MaxRetryAttempts},
	}
	for _, item := range checks {
		if item.value < item.min || item.value > item.max {
			return fmt.Errorf("%s", msg.SettingsRangeError(item.name, item.min, item.max))
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

func envStringConfigValue(envName string, fallback string) string {
	return stringConfigValue(os.Getenv(envName), fallback)
}

func writeConfigFile(sCfg ServerConfig) error {
	if err := os.MkdirAll("data", 0755); err != nil {
		return err
	}
	content := fmt.Sprintf(`[opensync]
bind=%s
port=%d
expires=%d
log_level=%d
console_level=%d
log_save=%d
task_save=%d
task_timeout=%d
copy_concurrency=%d
scan_concurrency=%d
max_retries=%d
trusted_proxies=%s
	`,
		sCfg.Bind,
		sCfg.Port,
		sCfg.Expires,
		sCfg.LogLevel,
		sCfg.ConsoleLevel,
		sCfg.LogSave,
		sCfg.TaskSave,
		sCfg.Timeout,
		sCfg.CopyConcurrency,
		sCfg.ScanConcurrency,
		sCfg.MaxRetries,
		strings.Join(sCfg.TrustedProxies, ","),
	)
	tmpFile, err := os.CreateTemp("data", "config.ini.*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmpFile.WriteString(content); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Chmod(0644); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, "data/config.ini"); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func intConfigValue(value string, fallback int, key string) int {
	i, err := strconv.Atoi(value)
	if err != nil {
		log.Printf("配置项 %s=%q 不是有效整数，将使用默认值 %d", key, value, fallback)
		return fallback
	}
	return i
}

func stringConfigValue(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
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
