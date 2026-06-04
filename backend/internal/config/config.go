package config

import (
	"bufio"
	"log"
	"opensync/pkg/crypto"
	"os"
	"strconv"
	"strings"
)

// ServerConfig holds server configuration
type ServerConfig struct {
	Port         int
	Expires      int
	LogLevel     int
	ConsoleLevel int
	LogSave      int
	TaskSave     int
	Timeout      int
	PasswdStr    string
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

var sysConfig *Config

// GetPasswordStr gets or generates the encryption secret key
func GetPasswordStr() string {
	return crypto.ReadOrSetFile("data/secret.key", crypto.GeneratePassword(256), false)
}

// GetConfig returns the global config (singleton)
func GetConfig() *Config {
	if sysConfig != nil {
		return sysConfig
	}
	passwdStr := GetPasswordStr()
	dbname := "data/openSync.db"

	sCfg := ServerConfig{
		Port:         8023,
		Expires:      2,
		LogLevel:     1,
		ConsoleLevel: 2,
		LogSave:      7,
		TaskSave:     0,
		Timeout:      72,
		PasswdStr:    passwdStr,
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
		}
	} else {
		// Read from environment variables
		if v := os.Getenv("OPENSYNC_PORT"); v != "" {
			if i, err := strconv.Atoi(v); err == nil {
				sCfg.Port = i
			}
		}
		if v := os.Getenv("OPENSYNC_EXPIRES"); v != "" {
			if i, err := strconv.Atoi(v); err == nil {
				sCfg.Expires = i
			}
		}
		if v := os.Getenv("OPENSYNC_LOG_LEVEL"); v != "" {
			if i, err := strconv.Atoi(v); err == nil {
				sCfg.LogLevel = i
			}
		}
		if v := os.Getenv("OPENSYNC_CONSOLE_LEVEL"); v != "" {
			if i, err := strconv.Atoi(v); err == nil {
				sCfg.ConsoleLevel = i
			}
		}
		if v := os.Getenv("OPENSYNC_LOG_SAVE"); v != "" {
			if i, err := strconv.Atoi(v); err == nil {
				sCfg.LogSave = i
			}
		}
		if v := os.Getenv("OPENSYNC_TASK_SAVE"); v != "" {
			if i, err := strconv.Atoi(v); err == nil {
				sCfg.TaskSave = i
			}
		}
		if v := os.Getenv("OPENSYNC_TASK_TIMEOUT"); v != "" {
			if i, err := strconv.Atoi(v); err == nil {
				sCfg.Timeout = i
			}
		}
	}

	sysConfig = &Config{
		DB:     DBConfig{DBName: dbname},
		Server: sCfg,
	}
	return sysConfig
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
