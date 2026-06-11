package service

import (
	"fmt"
	"io"
)

func readAllWithLimit(reader io.Reader, limit int64) ([]byte, error) {
	// Read at most limit+1 bytes so callers can detect oversized responses while
	// keeping memory use bounded by the configured response cap.
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response body exceeds %d bytes", limit)
	}
	return data, nil
}
