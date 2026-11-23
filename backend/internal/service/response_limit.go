package service

import (
	"errors"
	"fmt"
	"io"
)

const maxDrainBodyBytes = 4 << 10

// ErrResponseBodyTooLarge is returned when an HTTP response body exceeds the
// configured byte cap.
var ErrResponseBodyTooLarge = errors.New("response body exceeds limit")

type responseBodyTooLargeError struct {
	limit int64
}

func (e responseBodyTooLargeError) Error() string {
	return fmt.Sprintf("response body exceeds %d bytes", e.limit)
}

func (e responseBodyTooLargeError) Unwrap() error {
	return ErrResponseBodyTooLarge
}

func readAllWithLimit(reader io.Reader, limit int64) ([]byte, error) {
	// Read at most limit+1 bytes so callers can detect oversized responses while
	// keeping memory use bounded by the configured response cap.
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, responseBodyTooLargeError{limit: limit}
	}
	return data, nil
}

// drainAndCloseBody drains a small bounded amount of any remaining response
// body before closing so net/http can reuse persistent keep-alive connections
// without ever reading an unbounded stream.
func drainAndCloseBody(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, maxDrainBodyBytes))
	_ = body.Close()
}

// capReader errors as soon as more than limit bytes have been pulled, so a
// streaming JSON decoder cannot buffer an unbounded AList payload. Responses of
// exact length limit succeed cleanly; only limit+1 bytes triggers
// ErrResponseBodyTooLarge.
type capReader struct {
	r     io.Reader
	n     int64
	limit int64
}

func (c *capReader) Read(p []byte) (int, error) {
	if c.limit <= 0 {
		n, err := c.r.Read(p)
		c.n += int64(n)
		return n, err
	}
	if c.n > c.limit {
		return 0, responseBodyTooLargeError{limit: c.limit}
	}
	if len(p) == 0 {
		return 0, nil
	}
	if c.n == c.limit {
		var probe [1]byte
		n, err := c.r.Read(probe[:])
		if n > 0 {
			c.n += int64(n)
			return 0, responseBodyTooLargeError{limit: c.limit}
		}
		return 0, err
	}
	remain := c.limit - c.n
	if int64(len(p)) > remain {
		p = p[:remain]
	}
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
