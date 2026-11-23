package service

import (
	"errors"
	"opensync/internal/config"
	"opensync/internal/mapper"
	"sync"
	"time"
)

const (
	maxQueuedCopyItems     = 5000
	retryTaskItemBatchSize = 500
	persistBatchSize       = 100
	persistFlushInterval   = 500 * time.Millisecond
	maxScanListRetries     = 2
)

var errScanAborted = errors.New("scan aborted")

var persistJobTaskItems = mapper.AddJobTaskItemMany
var persistJobTaskItemsMu sync.RWMutex
var forEachJobTaskItemsByStatuses = mapper.ForEachJobTaskItemsByStatuses
var countJobTaskItemsByStatuses = mapper.CountJobTaskItemsByStatuses
var copyRetryDelay = defaultCopyRetryDelay
var scanListRetryDelay = defaultScanListRetryDelay

func callPersistJobTaskItems(items []map[string]interface{}) error {
	persistJobTaskItemsMu.RLock()
	fn := persistJobTaskItems
	persistJobTaskItemsMu.RUnlock()
	return fn(items)
}

func replacePersistJobTaskItems(fn func([]map[string]interface{}) error) func() {
	persistJobTaskItemsMu.Lock()
	previous := persistJobTaskItems
	persistJobTaskItems = fn
	persistJobTaskItemsMu.Unlock()
	return func() {
		persistJobTaskItemsMu.Lock()
		persistJobTaskItems = previous
		persistJobTaskItemsMu.Unlock()
	}
}

type taskRuntimeLimits struct {
	CopyConcurrency int
	ScanConcurrency int
	MaxRetries      int
}

func runtimeTaskLimits() taskRuntimeLimits {
	return taskRuntimeLimitsFromServer(config.GetConfig().Server)
}

func taskRuntimeLimitsFromServer(server config.ServerConfig) taskRuntimeLimits {
	return taskRuntimeLimits{
		CopyConcurrency: intInRangeOrDefault(
			server.CopyConcurrency,
			config.MinCopyConcurrency,
			config.MaxCopyConcurrency,
			config.DefaultCopyConcurrency,
		),
		ScanConcurrency: intInRangeOrDefault(
			server.ScanConcurrency,
			config.MinScanConcurrency,
			config.MaxScanConcurrency,
			config.DefaultScanConcurrency,
		),
		MaxRetries: intInRangeOrDefault(
			server.MaxRetries,
			config.MinMaxRetries,
			config.MaxRetryAttempts,
			config.DefaultMaxRetries,
		),
	}
}

func intInRangeOrDefault(value, minValue, maxValue, defaultValue int) int {
	if value < minValue {
		return defaultValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
