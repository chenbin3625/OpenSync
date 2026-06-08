package service

import (
	"errors"
	"opensync/internal/config"
	"opensync/internal/mapper"
)

const (
	defaultScanConcurrency       = 8
	maxScanConcurrency           = 20
	defaultCopyConcurrency       = 5
	maxCopyConcurrency           = 100
	maxQueuedCopyItems           = 5000
	defaultRealtimeFinishedItems = 1000
	maxRealtimeFinishedItems     = 50000
	maxPersistTaskItemBatch      = 500
	retryTaskItemBatchSize       = 500
	defaultMaxRetries            = 0
	maxCopyRetries               = 10
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
		CopyConcurrency:       intInRangeOrDefault(server.CopyConcurrency, 1, maxCopyConcurrency, defaultCopyConcurrency),
		ScanConcurrency:       intInRangeOrDefault(server.ScanConcurrency, 1, maxScanConcurrency, defaultScanConcurrency),
		RealtimeFinishedItems: intInRangeOrDefault(server.RealtimeFinishedItems, 100, maxRealtimeFinishedItems, defaultRealtimeFinishedItems),
		MaxRetries:            intInRangeOrDefault(server.MaxRetries, 0, maxCopyRetries, defaultMaxRetries),
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
