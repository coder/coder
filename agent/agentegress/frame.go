package agentegress

import (
	"encoding/binary"
	"io"

	"golang.org/x/xerrors"
)

// maxFrameSize is the largest payload a 2-byte length prefix can carry.
// UDP datagrams and DNS-over-TCP messages share this limit by construction.
const maxFrameSize = 65535

// writeFrame writes payload with a 2-byte big-endian length prefix, the
// framing used both by DNS over TCP (RFC 7766) and by the exit node's UDP
// and DNS CONNECT streams. The length and payload are written in one call
// so concurrent writers that serialize on a mutex never interleave.
func writeFrame(w io.Writer, payload []byte) error {
	if len(payload) > maxFrameSize {
		return xerrors.Errorf("frame of %d bytes exceeds %d", len(payload), maxFrameSize)
	}
	buf := make([]byte, 2+len(payload))
	// #nosec G115 -- bounded by maxFrameSize above.
	binary.BigEndian.PutUint16(buf, uint16(len(payload)))
	copy(buf[2:], payload)
	_, err := w.Write(buf)
	return err
}

// readFrame reads one length-prefixed payload. A zero-length frame is
// returned as an empty, non-nil slice.
func readFrame(r io.Reader) ([]byte, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint16(hdr[:])
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}
