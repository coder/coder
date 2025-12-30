// Package codec implements the wire format for agent <-> boundary communication.
//
// Wire Format:
//   - 4 bits: big-endian tag
//   - 28 bits: big-endian length of the protobuf data
//   - length bytes: encoded protobuf data
//
// Note that while there are 28 bits for the length, in practice the messages should
// be much smaller. 28 bits is chosen because sizing down the header to 16 bits would
// sacrifice future flexibility. For example, a 3 bit tag and 13 bit length doesn't
// have much length headroom should the batching strategy change.
package codec

import (
	"encoding/binary"
	"io"

	"golang.org/x/xerrors"
)

type Tag uint8

const (
	// TagV1 identifies the first revision of the protocol.
	TagV1 Tag = 1
)

const (
	// DataLength is the number of bits used for the length of encoded protobuf data.
	DataLength = 28

	// tagLength is the number of bits used for the tag.
	tagLength = 4

	// MaxMessageSize is the practical maximum size of the protobuf messages
	// sent over the wire. While the wire format allows for messages much larger
	// than this, practically they are not expected to be.
	MaxMessageSize = 1 << 15
)

var ErrTooLarge = xerrors.New("message too large")

// WriteFrame writes a framed message with the given tag and data. The tag must
// fit in 4 bits (0-15), and data must not exceed 2^DataLength in length.
func WriteFrame(w io.Writer, tag Tag, data []byte) error {
	if len(data) > 1<<DataLength {
		return xerrors.Errorf("data too large: %d bytes", len(data))
	}

	var header uint32
	//nolint:gosec // The length check above ensures there's no overflow.
	header |= uint32(len(data))
	header |= uint32(tag) << DataLength

	if err := binary.Write(w, binary.BigEndian, header); err != nil {
		return xerrors.Errorf("write header error: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return xerrors.Errorf("write data error: %w", err)
	}

	return nil
}

// ReadFrame reads a framed message, returning the decoded tag and data. If the
// message size exceeds maxSize, ErrTooLarge is returned. The provided buf is
// used if it has sufficient capacity; otherwise a new buffer is allocated. To
// reuse the buffer across calls, pass in the returned data slice:
//
//	buf := make([]byte, initialSize)
//	for {
//	    _, buf, _ = ReadFrame(r, maxSize, buf)
//	}
func ReadFrame(r io.Reader, maxSize uint32, buf []byte) (Tag, []byte, error) {
	var header uint32
	if err := binary.Read(r, binary.BigEndian, &header); err != nil {
		return 0, nil, xerrors.Errorf("read header error: %w", err)
	}

	length := header & 0x0FFFFFFF
	const tagMask = (1 << tagLength) - 1 // 0x0F
	shifted := (header >> DataLength) & tagMask
	if shifted > tagMask {
		// This is really only here to satisfy the gosec linter. We know from above that
		// shifted <= tagMask.
		return 0, nil, xerrors.Errorf("invalid tag: %d", shifted)
	}
	tag := Tag(shifted)

	if length > maxSize {
		return 0, nil, ErrTooLarge
	}

	if cap(buf) < int(length) {
		buf = make([]byte, length)
	} else {
		buf = buf[:length:cap(buf)]
	}

	if _, err := io.ReadFull(r, buf[:length]); err != nil {
		return 0, nil, xerrors.Errorf("read full error: %w", err)
	}

	return tag, buf[:length], nil
}
