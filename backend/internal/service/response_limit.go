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

// capReader errors as soon as more than limit bytes have been pulled, so a
// streaming JSON decoder cannot buffer an unbounded AList payload.
type capReader struct {
	r     io.Reader
	n     int64
	limit int64
}

func (c *capReader) Read(p []byte) (int, error) {
	if c.limit > 0 && c.n >= c.limit {
		return 0, fmt.Errorf("response body exceeds %d bytes", c.limit)
	}
	if c.limit > 0 {
		remain := c.limit - c.n
		if int64(len(p)) > remain {
			p = p[:remain]
		}
	}
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
