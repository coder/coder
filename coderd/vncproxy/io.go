package vncproxy

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net"

	"golang.org/x/xerrors"
)

// readExact reads exactly n bytes from r, blocking until they arrive
// or the read errors. It returns a fresh slice owned by the caller.
func readExact(r io.Reader, n int) ([]byte, error) {
	if n < 0 {
		return nil, xerrors.Errorf("readExact: negative length %d", n)
	}
	if n == 0 {
		return nil, nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// forwardExact reads exactly n bytes from src and writes them to dst.
// The bytes are streamed in copyBufSize chunks so a multi-MB
// FramebufferUpdate body does not balloon the heap. A short read or
// write returns the underlying error wrapped with context.
func forwardExact(src io.Reader, dst io.Writer, n int) error {
	if n < 0 {
		return xerrors.Errorf("forwardExact: negative length %d", n)
	}
	if n == 0 {
		return nil
	}
	const copyBufSize = 32 << 10
	buf := make([]byte, copyBufSize)
	remaining := n
	for remaining > 0 {
		chunk := remaining
		if chunk > copyBufSize {
			chunk = copyBufSize
		}
		if _, err := io.ReadFull(src, buf[:chunk]); err != nil {
			return xerrors.Errorf("read %d/%d bytes: %w", n-remaining, n, err)
		}
		if _, err := dst.Write(buf[:chunk]); err != nil {
			return xerrors.Errorf("write %d/%d bytes: %w", n-remaining, n, err)
		}
		remaining -= chunk
	}
	return nil
}

// drainExact reads exactly n bytes from src and discards them. Used
// when dropping a CutText body without forwarding it.
func drainExact(src io.Reader, n int) error {
	if n < 0 {
		return xerrors.Errorf("drainExact: negative length %d", n)
	}
	if n == 0 {
		return nil
	}
	if _, err := io.CopyN(io.Discard, src, int64(n)); err != nil {
		return err
	}
	return nil
}

// isMessageBoundaryEOF reports whether err signals a clean close at the
// start of the next message. We accept io.EOF (the peer half-closed
// after a complete message) plus the cancellation and closed-pipe
// errors that surface when a Coder dashboard tab disconnects mid-idle:
//
//   - context.Canceled / context.DeadlineExceeded: the WebSocket
//     handler's request context was canceled. The current read had
//     not consumed any message bytes yet, so resuming would not
//     desync.
//   - net.ErrClosed: the connection was closed by us (typically as
//     part of the same shutdown that canceled the context).
//
// Any of these observed *mid-message* would still desync the parser,
// but readExact returns the underlying io.ReadFull error which wraps
// io.ErrUnexpectedEOF for short reads. We deliberately do not match
// io.ErrUnexpectedEOF here so that case still surfaces as an error.
func isMessageBoundaryEOF(err error) bool {
	switch {
	case errors.Is(err, io.EOF):
		return true
	case errors.Is(err, context.Canceled):
		return true
	case errors.Is(err, context.DeadlineExceeded):
		return true
	case errors.Is(err, net.ErrClosed):
		return true
	}
	return false
}

// cutTextPayloadLen interprets the 4-byte length prefix of a CutText
// message and returns the number of bytes the parser must consume from
// the wire after the header. The value is signed on the wire because
// the extended-clipboard pseudo-encoding repurposes the high bit as a
// format flag, but we always drop the entire payload regardless.
//
// math.MinInt32 is rejected because negating it overflows back to
// itself, which would let a malicious peer pin us against the
// maxCutTextPayload cap and bypass the bound.
func cutTextPayloadLen(buf []byte) (int, error) {
	if len(buf) != 4 {
		return 0, xerrors.Errorf("length prefix is %d bytes, want 4", len(buf))
	}
	raw := int32(binary.BigEndian.Uint32(buf)) //nolint:gosec // RFB CutText length is a signed 32-bit integer per RFC 6143
	if raw == math.MinInt32 {
		return 0, xerrors.Errorf("length %d cannot be negated safely", raw)
	}
	n := raw
	if n < 0 {
		n = -n
	}
	if int(n) > maxCutTextPayload {
		return 0, xerrors.Errorf("payload %d exceeds cap %d", n, maxCutTextPayload)
	}
	return int(n), nil
}
