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

	// Only codes that can result from transient peer or transport conditions
	// are safe to retry. Protocol failures must remain terminal.
	var streamErr http2.StreamError
	if errors.As(err, &streamErr) && isTransientHTTP2Error(streamErr.Code) {
		return true
	}
	var streamErrPtr *http2.StreamError
	if errors.As(err, &streamErrPtr) && isTransientHTTP2Error(streamErrPtr.Code) {
		return true
	}
	var goAwayErr http2.GoAwayError
	if errors.As(err, &goAwayErr) && isTransientHTTP2Error(goAwayErr.ErrCode) {
		return true
	}
	var goAwayErrPtr *http2.GoAwayError
	return errors.As(err, &goAwayErrPtr) && isTransientHTTP2Error(goAwayErrPtr.ErrCode)
}

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
