// Package xnet classifies network transport errors.
package xnet

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"

	"golang.org/x/net/http2"
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
	if errors.As(err, &opErr) {
		return true
	}

	// net/http bundles its own HTTP/2 implementation with unexported error
	// types, and net/http/h2_error.go bridges only the struct form of a
	// stream error to this package's type. A pointer target never matches,
	// and GOAWAY errors have no such bridge.
	var streamErr http2.StreamError
	return errors.As(err, &streamErr) && isTransientHTTP2Error(streamErr.Code)
}

// isTransientHTTP2Error reports whether an HTTP/2 error code can result from a
// transient peer or transport condition. Deterministic protocol failures stay
// terminal so a malformed consumer response is not retried.
func isTransientHTTP2Error(code http2.ErrCode) bool {
	switch code {
	case http2.ErrCodeNo,
		http2.ErrCodeInternal,
		http2.ErrCodeRefusedStream,
		http2.ErrCodeCancel,
		http2.ErrCodeEnhanceYourCalm:
		return true
	default:
		return false
	}
}
