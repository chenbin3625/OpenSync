package util

import (
	"fmt"
	"math"
	"time"
)

// ConvertBytes converts bytes to human-readable string
func ConvertBytes(val int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	size := float64(val)
	absSize := math.Abs(size)
	for absSize >= 1024 && i < len(units)-1 {
		size /= 1024
		absSize /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d %s", val, units[i])
	}
	return fmt.Sprintf("%.2f %s", size, units[i])
}

// ConvertSeconds converts seconds to hours, minutes, seconds
func ConvertSeconds(seconds int) (int, int, int) {
	hours := seconds / 3600
	remaining := seconds % 3600
	minutes := remaining / 60
	secs := remaining % 60
	return hours, minutes, secs
}

// StampToTime converts unix timestamp to formatted string
func StampToTime(stamp int64) string {
	return time.Unix(stamp, 0).Format("2006-01-02 15:04:05")
}

// TimeToStamp converts formatted time string to unix timestamp
func TimeToStamp(timeStr string) (int64, error) {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", timeStr, time.Local)
	if err != nil {
		return 0, err
	}
	return t.Unix(), nil
}
