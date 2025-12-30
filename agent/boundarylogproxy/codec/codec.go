// Package codec implements the wire format for agent <-> boundary communication.
//
// Wire Format:
//   - 8 bits: big-endian tag
//   - 24 bits: big-endian length of the protobuf data (bit usage depends on tag)
//   - length bytes: encoded protobuf data
//
// Note that while there are 24 bits available for the length, the actual maximum
// length depends on the tag. For TagV1, only 15 bits are used (MaxMessageSizeV1).
package codec

import (
	"encoding/binary"
	"io"

	"golang.org/x/xerrors"
)

type Tag uint8

const (
	// TagV1 identifies the first revision of the protocol. This version has a maximum
	// data length of MaxMessageSizeV1.
	TagV1 Tag = 1
)

const (
	// DataLength is the number of bits used for the length of encoded protobuf data.
	DataLength = 24

	// tagLength is the number of bits used for the tag.
	tagLength = 8

	// MaxMessageSizeV1 is the maximum size of the encoded protobuf messages sent
	// over the wire for the TagV1 tag. While the wire format allows 24 bits for
	// length, TagV1 only uses 15 bits.
	MaxMessageSizeV1 uint32 = 1 << 15
)

var ErrMessageTooLarge = xerrors.New("message too large")

// WriteFrame writes a framed message with the given tag and data. The data
// must not exceed 2^DataLength in length.
func WriteFrame(w io.Writer, tag Tag, data []byte) error {
	var maxSize uint32
	switch tag {
	case TagV1:
		maxSize = MaxMessageSizeV1
	default:
		return xerrors.Errorf("unsupported tag: %d", tag)
	}

	if len(data) > int(maxSize) {
		return xerrors.Errorf("data too large for tag %d: %d > %d", tag, len(data), maxSize)
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
// message size exceeds MaxMessageSizeV1, ErrMessageTooLarge is returned. The
// provided buf is used if it has sufficient capacity; otherwise a new buffer is
// allocated. To reuse the buffer across calls, pass in the returned data slice:
//
//	buf := make([]byte, initialSize)
//	for {
//	    _, buf, _ = ReadFrame(r, buf)
//	}
func ReadFrame(r io.Reader, buf []byte) (Tag, []byte, error) {
	var header uint32
	if err := binary.Read(r, binary.BigEndian, &header); err != nil {
		return 0, nil, xerrors.Errorf("read header error: %w", err)
	}

	const lengthMask = (1 << DataLength) - 1
	length := header & lengthMask
	const tagMask = (1 << tagLength) - 1 // 0xFF
	shifted := (header >> DataLength) & tagMask
	if shifted > tagMask {
		// This is really only here to satisfy the gosec linter. We know from above that
		// shifted <= tagMask.
		return 0, nil, xerrors.Errorf("invalid tag: %d", shifted)
	}
	tag := Tag(shifted)

	var maxSize uint32
	switch tag {
	case TagV1:
		maxSize = MaxMessageSizeV1
	default:
		return 0, nil, xerrors.Errorf("unsupported tag: %d", tag)
	}

	if length > maxSize {
		return 0, nil, ErrMessageTooLarge
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
