package exitnode

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/quartz"
)

const (
	// DefaultUDPIdleTimeout closes a udp stream after this long without a
	// datagram in either direction.
	DefaultUDPIdleTimeout = 60 * time.Second

	// maxFramePayload is the largest payload a frame can carry. The length
	// prefix is 16 bits, so this is also the largest value it can encode.
	maxFramePayload = 65535
	frameHeaderLen  = 2
)

// readFrame reads one length-prefixed frame into buf and returns the payload
// slice, which aliases buf. buf must be at least maxFramePayload bytes.
//
// Frames are a 2-byte big-endian payload length followed by the payload.
// This is the DNS-over-TCP framing (RFC 1035 section 4.2.2) reused for udp
// streams so one framing serves both.
func readFrame(r io.Reader, buf []byte) ([]byte, error) {
	var hdr [frameHeaderLen]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := int(binary.BigEndian.Uint16(hdr[:]))
	if n > maxFramePayload || n > len(buf) {
		return nil, xerrors.Errorf("frame of %d bytes exceeds maximum %d", n, min(maxFramePayload, len(buf)))
	}
	if _, err := io.ReadFull(r, buf[:n]); err != nil {
		return nil, xerrors.Errorf("read frame payload: %w", err)
	}
	return buf[:n], nil
}

// frameWriter writes length-prefixed frames. Each frame is written with one
// Write call under a mutex so concurrent writers never interleave.
type frameWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (f *frameWriter) write(payload []byte) error {
	if len(payload) > maxFramePayload {
		return xerrors.Errorf("payload of %d bytes exceeds maximum %d", len(payload), maxFramePayload)
	}
	frame := make([]byte, frameHeaderLen+len(payload))
	// #nosec G115 - len(payload) is bounded by maxFramePayload above.
	binary.BigEndian.PutUint16(frame, uint16(len(payload)))
	copy(frame[frameHeaderLen:], payload)
	f.mu.Lock()
	defer f.mu.Unlock()
	_, err := f.w.Write(frame)
	return err
}

// relayDatagrams carries framed datagrams between client and a connected UDP
// upstream until either side fails, ctx is canceled, or idle elapses with no
// traffic. Both connections are closed on return. It returns payload bytes
// copied from upstream to client (in) and client to upstream (out).
//
// A datagram the upstream socket refuses to send, for example because it is
// larger than the path allows, ends the stream: the client has no other way
// to learn the datagram was lost.
func relayDatagrams(ctx context.Context, clock quartz.Clock, client, upstream net.Conn, idle time.Duration) (bytesIn, bytesOut int64) {
	closeBoth := func() {
		_ = client.Close()
		_ = upstream.Close()
	}
	stop := context.AfterFunc(ctx, closeBoth)
	defer stop()

	var timerMu sync.Mutex
	timer := clock.AfterFunc(idle, closeBoth, "relayDatagrams", "idle")
	defer timer.Stop()
	touch := func() {
		timerMu.Lock()
		defer timerMu.Unlock()
		timer.Reset(idle)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		// Closing the upstream unblocks the other goroutine's Read.
		defer upstream.Close()
		buf := make([]byte, maxFramePayload)
		for {
			payload, err := readFrame(client, buf)
			if err != nil {
				return
			}
			touch()
			if _, err := upstream.Write(payload); err != nil {
				return
			}
			bytesOut += int64(len(payload))
		}
	}()
	go func() {
		defer wg.Done()
		// Closing the client unblocks the other goroutine's readFrame.
		defer client.Close()
		fw := &frameWriter{w: client}
		buf := make([]byte, maxFramePayload)
		for {
			n, err := upstream.Read(buf)
			if err != nil {
				return
			}
			touch()
			if err := fw.write(buf[:n]); err != nil {
				return
			}
			bytesIn += int64(n)
		}
	}()
	wg.Wait()
	return bytesIn, bytesOut
}
