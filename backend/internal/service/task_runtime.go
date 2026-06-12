package service

import (
	"errors"
	"opensync/internal/config"
	"opensync/internal/mapper"
)

const (
	maxQueuedCopyItems      = 5000
	maxPersistTaskItemBatch = 500
	retryTaskItemBatchSize  = 500
)

var errScanAborted = errors.New("scan aborted")

var persistJobTaskItems = mapper.AddJobTaskItemMany
var forEachJobTaskItemsByStatuses = mapper.ForEachJobTaskItemsByStatuses
var countJobTaskItemsByStatuses = mapper.CountJobTaskItemsByStatuses
var copyRetryDelay = defaultCopyRetryDelay

type taskRuntimeLimits struct {
	CopyConcurrency       int
	ScanConcurrency       int
	RealtimeFinishedItems int
	MaxRetries            int
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
		RealtimeFinishedItems: intInRangeOrDefault(
			server.RealtimeFinishedItems,
			config.MinRealtimeFinishedItems,
			config.MaxRealtimeFinishedItems,
			config.DefaultRealtimeFinishedItems,
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
