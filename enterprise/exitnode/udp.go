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

// readFrame reads a 2-byte length-prefixed frame into buf.
func readFrame(r io.Reader, buf []byte) ([]byte, error) {
	var hdr [frameHeaderLen]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := int(binary.BigEndian.Uint16(hdr[:]))
	if n > len(buf) {
		return nil, xerrors.Errorf("frame of %d bytes exceeds maximum %d", n, len(buf))
	}
	if _, err := io.ReadFull(r, buf[:n]); err != nil {
		return nil, xerrors.Errorf("read frame payload: %w", err)
	}
	return buf[:n], nil
}

// frameWriter serializes length-prefixed frames.
type frameWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (f *frameWriter) write(payload []byte) error {
	if len(payload) > maxFramePayload {
		return xerrors.Errorf("payload of %d bytes exceeds maximum %d", len(payload), maxFramePayload)
	}
	// #nosec G115 - len(payload) is bounded by maxFramePayload above.
	frame := append(binary.BigEndian.AppendUint16(make([]byte, 0, frameHeaderLen+len(payload)), uint16(len(payload))), payload...)
	f.mu.Lock()
	defer f.mu.Unlock()
	_, err := f.w.Write(frame)
	return err
}

// relayDatagrams carries framed datagrams until failure, cancellation, or idle
// timeout and returns payload bytes in each direction.
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
	wg.Go(func() {
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
	})
	wg.Go(func() {
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
	})
	wg.Wait()
	return bytesIn, bytesOut
}
