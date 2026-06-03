package util

import (
	"fmt"
	"time"
)

// ConvertBytes converts bytes to human-readable string
func ConvertBytes(val int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	fVal := float64(val)
	for i < len(units) {
		i++
		if fVal < float64(pow1024(i+1)) {
			return fmt.Sprintf("%.2f %s", fVal/float64(pow1024(i)), units[i])
		}
	}
	return fmt.Sprintf("%.2f %s", fVal/float64(pow1024(i-1)), units[i-1])
}

func pow1024(n int) int64 {
	var result int64 = 1
	for j := 0; j < n; j++ {
		result *= 1024
	}
	return result
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
