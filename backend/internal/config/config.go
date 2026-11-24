package config

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"opensync/internal/msg"
	"opensync/pkg/crypto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// ServerConfig holds server configuration
type ServerConfig struct {
	Bind              string
	Port              int
	Expires           int
	LogLevel          int
	ConsoleLevel      int
	LogSave           int
	TaskSave          int
	Timeout           int
	CopyConcurrency   int
	ScanConcurrency   int
	MaxRetries        int
	PasswdStr         string
	AllowInsecureHTTP bool
	AllowRemoteInit   bool
	// TrustedProxies lists CIDR ranges (or bare IPs) whose X-Forwarded-Proto
	// header may be honored when deciding whether to mark the auth cookie
	// Secure behind a TLS-terminating reverse proxy.
	TrustedProxies []string
	// TLSCertFile and TLSKeyFile enable HTTPS/HTTP2 and opportunistic HTTP3.
	TLSCertFile string
	TLSKeyFile  string
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
	defaultBind         = "127.0.0.1"
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

// GetPasswordStr gets or generates the encryption secret key. Persistence is
// mandatory: a key that only exists in memory would rotate on restart and
// invalidate every stored cookie and encrypted token, so a write failure is
// fatal at startup.
func GetPasswordStr() string {
	keyPath := DataPath("secret.key")
	key, err := crypto.ReadOrSetFile(keyPath, crypto.GeneratePassword(256), false)
	if err != nil {
		log.Fatalf("Failed to persist %s: %v", keyPath, err)
	}
	return key
}

// DataPath returns a path under the configured persistent data directory.
func DataPath(parts ...string) string {
	root := strings.TrimSpace(os.Getenv("OPENSYNC_DATA_DIR"))
	if root == "" {
		root = "data"
	}
	pathParts := append([]string{root}, parts...)
	return filepath.Join(pathParts...)
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
	dbname := DataPath("openSync.db")

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

	if _, err := os.Stat(DataPath("config.ini")); err == nil {
		// Read config.ini
		iniMap := readINI(DataPath("config.ini"))
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
	}
	// Deployment environment variables override persisted config values.
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
	if value := strings.TrimSpace(os.Getenv("OPENSYNC_TRUSTED_PROXIES")); value != "" {
		sCfg.TrustedProxies = parseTrustedProxies(value)
	}
	sCfg.TLSCertFile = envStringConfigValue("OPENSYNC_TLS_CERT", sCfg.TLSCertFile)
	sCfg.TLSKeyFile = envStringConfigValue("OPENSYNC_TLS_KEY", sCfg.TLSKeyFile)
	sCfg.AllowInsecureHTTP = envBoolConfigValue("OPENSYNC_ALLOW_INSECURE_HTTP", false)
	sCfg.AllowRemoteInit = envBoolConfigValue("OPENSYNC_ALLOW_REMOTE_INIT", false)

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

func (s ServerConfig) TLSEnabled() bool {
	return strings.TrimSpace(s.TLSCertFile) != "" && strings.TrimSpace(s.TLSKeyFile) != ""
}

func (s ServerConfig) TLSPartiallyConfigured() bool {
	hasCert := strings.TrimSpace(s.TLSCertFile) != ""
	hasKey := strings.TrimSpace(s.TLSKeyFile) != ""
	return hasCert != hasKey
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

// SystemSettingsPatch describes an atomic partial update to runtime settings.
// Nil fields preserve the current value under configMu so concurrent requests
// modifying disjoint fields never overwrite each other.
type SystemSettingsPatch struct {
	Expires         *int
	TaskTimeout     *int
	TaskSave        *int
	CopyConcurrency *int
	ScanConcurrency *int
	MaxRetries      *int
}

func systemSettingsFromServer(s ServerConfig) SystemSettings {
	return SystemSettings{
		Expires:         s.Expires,
		TaskTimeout:     s.Timeout,
		TaskSave:        s.TaskSave,
		CopyConcurrency: s.CopyConcurrency,
		ScanConcurrency: s.ScanConcurrency,
		MaxRetries:      s.MaxRetries,
	}
}

// GetSystemSettings returns the runtime-editable settings.
func GetSystemSettings() SystemSettings {
	cfg := GetConfig()
	return systemSettingsFromServer(cfg.Server)
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
	if err := writeConfigKeys(runtimeSettingsValues(settings), runtimeConfigManagedKeys); err != nil {
		return err
	}
	nextServer := cfg.Server
	nextServer.Expires = settings.Expires
	nextServer.Timeout = settings.TaskTimeout
	nextServer.TaskSave = settings.TaskSave
	nextServer.CopyConcurrency = settings.CopyConcurrency
	nextServer.ScanConcurrency = settings.ScanConcurrency
	nextServer.MaxRetries = settings.MaxRetries
	sysConfig = &Config{
		DB:     cfg.DB,
		Server: nextServer,
	}
	return nil
}

// PatchSystemSettings atomically merges non-nil fields from patch onto the
// current runtime configuration under configMu, validates, persists only the
// modified runtime keys, and returns the updated settings snapshot.
func PatchSystemSettings(patch SystemSettingsPatch) (SystemSettings, error) {
	GetConfig()
	configMu.Lock()
	defer configMu.Unlock()

	cfg := sysConfig
	nextSettings := systemSettingsFromServer(cfg.Server)
	patchedKeys := make([]string, 0, len(runtimeConfigManagedKeys))
	patchedValues := make(map[string]string, len(runtimeConfigManagedKeys))

	if patch.Expires != nil {
		nextSettings.Expires = *patch.Expires
		patchedKeys = append(patchedKeys, "expires")
		patchedValues["expires"] = strconv.Itoa(*patch.Expires)
	}
	if patch.TaskSave != nil {
		nextSettings.TaskSave = *patch.TaskSave
		patchedKeys = append(patchedKeys, "task_save")
		patchedValues["task_save"] = strconv.Itoa(*patch.TaskSave)
	}
	if patch.TaskTimeout != nil {
		nextSettings.TaskTimeout = *patch.TaskTimeout
		patchedKeys = append(patchedKeys, "task_timeout")
		patchedValues["task_timeout"] = strconv.Itoa(*patch.TaskTimeout)
	}
	if patch.CopyConcurrency != nil {
		nextSettings.CopyConcurrency = *patch.CopyConcurrency
		patchedKeys = append(patchedKeys, "copy_concurrency")
		patchedValues["copy_concurrency"] = strconv.Itoa(*patch.CopyConcurrency)
	}
	if patch.ScanConcurrency != nil {
		nextSettings.ScanConcurrency = *patch.ScanConcurrency
		patchedKeys = append(patchedKeys, "scan_concurrency")
		patchedValues["scan_concurrency"] = strconv.Itoa(*patch.ScanConcurrency)
	}
	if patch.MaxRetries != nil {
		nextSettings.MaxRetries = *patch.MaxRetries
		patchedKeys = append(patchedKeys, "max_retries")
		patchedValues["max_retries"] = strconv.Itoa(*patch.MaxRetries)
	}

	if err := validateSystemSettings(nextSettings); err != nil {
		return SystemSettings{}, err
	}

	if len(patchedKeys) > 0 {
		if err := writeConfigKeys(patchedValues, patchedKeys); err != nil {
			return SystemSettings{}, err
		}
		nextServer := cfg.Server
		nextServer.Expires = nextSettings.Expires
		nextServer.Timeout = nextSettings.TaskTimeout
		nextServer.TaskSave = nextSettings.TaskSave
		nextServer.CopyConcurrency = nextSettings.CopyConcurrency
		nextServer.ScanConcurrency = nextSettings.ScanConcurrency
		nextServer.MaxRetries = nextSettings.MaxRetries
		sysConfig = &Config{
			DB:     cfg.DB,
			Server: nextServer,
		}
	}
	return nextSettings, nil
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

func envBoolConfigValue(envName string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envName))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

// runtimeConfigManagedKeys are the runtime-editable [opensync] keys persisted
// when settings are saved from the UI or API. Deployment parameters such as
// bind, port, trusted_proxies, and log levels are intentionally excluded so
// temporary environment overrides are never baked into config.ini.
var runtimeConfigManagedKeys = []string{
	"expires", "task_save", "task_timeout",
	"copy_concurrency", "scan_concurrency", "max_retries",
}

func runtimeSettingsValues(settings SystemSettings) map[string]string {
	return map[string]string{
		"expires":          strconv.Itoa(settings.Expires),
		"task_save":        strconv.Itoa(settings.TaskSave),
		"task_timeout":     strconv.Itoa(settings.TaskTimeout),
		"copy_concurrency": strconv.Itoa(settings.CopyConcurrency),
		"scan_concurrency": strconv.Itoa(settings.ScanConcurrency),
		"max_retries":      strconv.Itoa(settings.MaxRetries),
	}
}

// writeConfigKeys updates only the given [opensync] keys in data/config.ini
// while preserving deployment keys, comments, unknown keys, and any other
// sections verbatim. Duplicate [opensync] sections or duplicate entries for
// updated keys are merged cleanly so updated settings never revert on restart.
func writeConfigKeys(values map[string]string, orderedKeys []string) error {
	configPath := DataPath("config.ini")
	dataDir := filepath.Dir(configPath)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return err
	}
	pending := make(map[string]string, len(values))
	for k, v := range values {
		pending[k] = v
	}

	var preOpensync []string
	var opensyncBody []string
	var postOpensync []string
	seenOpensync := false
	inOpensync := false

	if existing, err := os.ReadFile(configPath); err == nil {
		raw := strings.ReplaceAll(string(existing), "\r\n", "\n")
		raw = strings.TrimSuffix(raw, "\n")
		var lines []string
		if raw != "" {
			lines = strings.Split(raw, "\n")
		}
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
				if trimmed == "[opensync]" {
					inOpensync = true
					seenOpensync = true
					continue
				}
				inOpensync = false
				if seenOpensync {
					postOpensync = append(postOpensync, line)
				} else {
					preOpensync = append(preOpensync, line)
				}
				continue
			}
			if inOpensync {
				if !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, ";") {
					if key, _, isKV := strings.Cut(trimmed, "="); isKV {
						key = strings.TrimSpace(key)
						if _, isManaged := values[key]; isManaged {
							if v, stillPending := pending[key]; stillPending {
								opensyncBody = append(opensyncBody, key+"="+v)
								delete(pending, key)
							}
							continue
						}
					}
				}
				opensyncBody = append(opensyncBody, line)
				continue
			}
			if seenOpensync {
				postOpensync = append(postOpensync, line)
			} else {
				preOpensync = append(preOpensync, line)
			}
		}
	}

	trailingBlankCount := 0
	for len(opensyncBody)-trailingBlankCount > 0 && strings.TrimSpace(opensyncBody[len(opensyncBody)-1-trailingBlankCount]) == "" {
		trailingBlankCount++
	}
	trailingBlanks := append([]string(nil), opensyncBody[len(opensyncBody)-trailingBlankCount:]...)
	opensyncBody = opensyncBody[:len(opensyncBody)-trailingBlankCount]

	for _, key := range orderedKeys {
		if v, ok := pending[key]; ok {
			opensyncBody = append(opensyncBody, key+"="+v)
			delete(pending, key)
		}
	}
	opensyncBody = append(opensyncBody, trailingBlanks...)

	var out []string
	out = append(out, preOpensync...)
	if !seenOpensync && len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
		out = append(out, "")
	}
	out = append(out, "[opensync]")
	out = append(out, opensyncBody...)
	if len(postOpensync) > 0 && len(trailingBlanks) == 0 {
		out = append(out, "")
	}
	out = append(out, postOpensync...)

	content := strings.Join(out, "\n") + "\n"

	tmpFile, err := os.CreateTemp(dataDir, "config.ini.*")
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
	if err := tmpFile.Sync(); err != nil {
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
	if err := os.Rename(tmpName, configPath); err != nil {
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
