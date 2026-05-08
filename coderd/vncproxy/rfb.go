// Package vncproxy implements a clipboard-aware byte pump for the noVNC
// desktop WebSocket. It parses the in-flight RFB stream just enough to
// identify and drop ClientCutText (C2S type 6) and ServerCutText (S2C
// type 3) messages while forwarding everything else byte-for-byte. It is
// the inspection backend for the `coder_dlp_policy.clipboard_access`
// gate; see coderd/workspaceapps/proxy.go workspaceAgentDesktop for the
// caller.
//
// The parser is purely a stream walker. It does not decode or render
// anything, but it must compute the byte length of every message
// precisely so the proxied stream stays framed.
//
// Encoding restriction: when this package is in use, the client's
// SetEncodings message is rewritten to drop encodings whose rectangle
// bodies would require a zlib stream to size (Tight, ZRLE, TightPNG,
// Zlib, ZlibHex, TRLE, JPEG, JRLE). The server falls back to the
// uncompressed encodings the parser can size on its own (Raw, CopyRect,
// RRE, CoRRE, Hextile). Pseudo-encodings whose payloads we cannot size
// are also dropped from the negotiation.
package vncproxy

import (
	"context"
	"errors"
	"io"

	"golang.org/x/xerrors"
)

// Direction identifies which leg of the RFB connection produced or
// dropped a message. The names are intentionally hyphenated ASCII so
// they can be embedded in connection_logs reason strings without
// tripping the repository's no-emdash lint.
type Direction int

const (
	// DirClientToServer is the client-to-server (C2S) leg of the RFB
	// connection. ClientCutText (type 6) flows in this direction.
	DirClientToServer Direction = iota
	// DirServerToClient is the server-to-client (S2C) leg.
	// ServerCutText (type 3) flows in this direction.
	DirServerToClient
)

// String renders a Direction for use in log messages and connection_logs
// reason strings. Returns ASCII, never Unicode arrows.
func (d Direction) String() string {
	switch d {
	case DirClientToServer:
		return "client-to-server"
	case DirServerToClient:
		return "server-to-client"
	default:
		return "unknown"
	}
}

// DropEvent describes a single CutText message that the proxy refused to
// forward.
type DropEvent struct {
	// Direction is the leg the dropped message was traveling on.
	Direction Direction
	// MessageType is the RFB message type byte. 6 for ClientCutText, 3
	// for ServerCutText.
	MessageType byte
	// PayloadLen is the byte length of the cut text payload (not
	// including the 8-byte CutText header). For extended-clipboard
	// messages the absolute value of the protocol's signed length
	// field is used.
	PayloadLen int
}

// maxCutTextPayload caps the size of a CutText payload the parser will
// drain before declaring the stream malformed. The base RFB cut text is
// text-only and rarely above a few KB; extended clipboard can carry
// images but a 16 MiB cap is well past anything legitimate while still
// bounding worst-case memory use during the drop.
const maxCutTextPayload = 16 << 20

// BicopyDropClipboard pumps bytes between client and server, dropping
// every ClientCutText (C2S type 6) and ServerCutText (S2C type 3)
// message it observes. Every other message is forwarded byte-for-byte.
// The C2S SetEncodings message (type 2) is rewritten to remove
// encodings whose body lengths require a zlib stream to compute.
//
// onDrop is invoked from the goroutine that observed the drop. It must
// be safe for concurrent use across the two directions if it shares
// state with the caller. A nil onDrop is treated as a no-op.
//
// Both connections are closed when the function returns. Returning nil
// indicates a clean EOF on both directions; a non-nil error indicates
// either a context cancellation or that the stream desynced and the
// parser had to abort to preserve framing.
func BicopyDropClipboard(
	ctx context.Context,
	client, server io.ReadWriteCloser,
	onDrop func(DropEvent),
) error {
	if onDrop == nil {
		onDrop = func(DropEvent) {}
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Close both sides on return so a stuck Read on either side unblocks.
	defer func() {
		_ = client.Close()
		_ = server.Close()
	}()

	// Cancel the parser if the caller's context is done before either
	// side hits EOF. This is harmless if both sides have already
	// finished cleanly.
	stopOnContext := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = client.Close()
			_ = server.Close()
		case <-stopOnContext:
		}
	}()
	defer close(stopOnContext)

	sess := newSession()

	// The RFB handshake is strictly sequential: the server speaks first
	// with ProtocolVersion, then the client replies, etc. We drive the
	// whole handshake here on the calling goroutine before fanning out
	// to two per-direction goroutines for the steady-state message
	// phase. Doing it this way avoids any cross-goroutine sequencing
	// during the most state-dense part of the protocol.
	//
	// The reads on each side are wrapped in a tracing reader so that
	// when we surface a desync error, the log line includes the running
	// byte offset and the most recent bytes seen on that side. That is
	// the only practical way to debug a parser desync against a live
	// VNC server because we never get to see the wire bytes again.
	clientTrace := newTracingReader(client)
	serverTrace := newTracingReader(server)
	handshakeClient := struct {
		io.Reader
		io.Writer
	}{Reader: clientTrace, Writer: client}
	handshakeServer := struct {
		io.Reader
		io.Writer
	}{Reader: serverTrace, Writer: server}
	if err := runHandshake(sess, handshakeClient, handshakeServer); err != nil {
		return formatTracedError("handshake", err, clientTrace, serverTrace)
	}

	type result struct {
		dir Direction
		err error
	}
	results := make(chan result, 2)

	go func() {
		err := pumpClientToServer(sess, clientTrace, server, onDrop)
		results <- result{dir: DirClientToServer, err: err}
	}()
	go func() {
		err := pumpServerToClient(sess, serverTrace, client, onDrop)
		results <- result{dir: DirServerToClient, err: err}
	}()

	// Wait for both goroutines to exit. The first non-EOF error wins.
	var firstErr error
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err == nil || errors.Is(r.err, io.EOF) || errors.Is(r.err, io.ErrUnexpectedEOF) || errors.Is(r.err, io.ErrClosedPipe) {
			// Treat any of these as a clean half-close; the other
			// goroutine will exit on the next Read once we Close
			// below.
			_ = client.Close()
			_ = server.Close()
			continue
		}
		trace := clientTrace
		if r.dir == DirServerToClient {
			trace = serverTrace
		}
		err := formatTracedError(r.dir.String(), r.err, trace, nil)
		if firstErr == nil {
			firstErr = err
		}
		// Force the other side to exit so we don't deadlock.
		_ = client.Close()
		_ = server.Close()
	}
	return firstErr
}

// formatTracedError builds a parser desync error message that embeds
// the byte offset and the most recent N bytes seen on the side(s)
// involved. trailing may be nil for non-handshake errors where only
// the primary side's trace is meaningful.
func formatTracedError(prefix string, err error, primary *tracingReader, trailing *tracingReader) error {
	off, tail := primary.snapshot()
	if trailing == nil {
		return xerrors.Errorf("%s: %w (offset=%d tail=%x)", prefix, err, off, tail)
	}
	off2, tail2 := trailing.snapshot()
	return xerrors.Errorf("%s: %w (client_off=%d client_tail=%x server_off=%d server_tail=%x)", prefix, err, off, tail, off2, tail2)
}
