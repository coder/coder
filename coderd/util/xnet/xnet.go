// Package xnet classifies network transport errors.
package xnet

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"
)

// IsTimeoutError reports whether err indicates the peer did not respond in
// time, including context deadlines and net.Error timeouts.
func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, syscall.ETIMEDOUT) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// IsConnectionError reports whether err indicates a failed or interrupted
// connection, such as refused dials, resets, and unexpected EOFs.
func IsConnectionError(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, net.ErrClosed) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.EPIPE) {
		return true
	}
	var opErr *net.OpError
	return errors.As(err, &opErr)
}
