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
	"google.golang.org/protobuf/proto"

	agentproto "github.com/coder/coder/v2/agent/proto"
)

type Tag uint8

const (
	// TagV1 identifies the first revision of the protocol. The payload is a
	// bare ReportBoundaryLogsRequest. This version has a maximum data length
	// of MaxMessageSizeV1.
	TagV1 Tag = 1

	// TagV2 identifies the second revision of the protocol. The payload is
	// a BoundaryMessage envelope. This version has a maximum data length of
	// MaxMessageSizeV2.
	TagV2 Tag = 2
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

	// MaxMessageSizeV2 is the maximum data length for TagV2.
	MaxMessageSizeV2 = MaxMessageSizeV1
)

var (
	// ErrMessageTooLarge is returned when the message exceeds the maximum size
	// allowed for the tag.
	ErrMessageTooLarge = xerrors.New("message too large")
	// ErrUnsupportedTag is returned when an unrecognized tag is encountered.
	ErrUnsupportedTag = xerrors.New("unsupported tag")
)

// WriteFrame writes a framed message with the given tag and data. The data
// must not exceed 2^DataLength in length.
func WriteFrame(w io.Writer, tag Tag, data []byte) error {
	maxSize, err := maxSizeForTag(tag)
	if err != nil {
		return err
	}

	if len(data) > int(maxSize) {
		return xerrors.Errorf("%w for tag %d: %d > %d", ErrMessageTooLarge, tag, len(data), maxSize)
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

	maxSize, err := maxSizeForTag(tag)
	if err != nil {
		return 0, nil, err
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

// maxSizeForTag returns the maximum payload size for the given tag.
func maxSizeForTag(tag Tag) (uint32, error) {
	switch tag {
	case TagV1:
		return MaxMessageSizeV1, nil
	case TagV2:
		return MaxMessageSizeV2, nil
	default:
		return 0, xerrors.Errorf("%w: %d", ErrUnsupportedTag, tag)
	}
}

// ReadMessage reads a framed message and unmarshals it based on tag. The
// returned buf should be passed back on the next call for buffer reuse.
func ReadMessage(r io.Reader, buf []byte) (proto.Message, []byte, error) {
	tag, data, err := ReadFrame(r, buf)
	if err != nil {
		return nil, data, err
	}

	var msg proto.Message
	switch tag {
	case TagV1:
		var req agentproto.ReportBoundaryLogsRequest
		if err := proto.Unmarshal(data, &req); err != nil {
			return nil, data, xerrors.Errorf("unmarshal TagV1: %w", err)
		}
		msg = &req
	case TagV2:
		var envelope BoundaryMessage
		if err := proto.Unmarshal(data, &envelope); err != nil {
			return nil, data, xerrors.Errorf("unmarshal TagV2: %w", err)
		}
		msg = &envelope
	default:
		// maxSizeForTag already rejects unknown tags during ReadFrame,
		// but handle it here for safety.
		return nil, data, xerrors.Errorf("%w: %d", ErrUnsupportedTag, tag)
	}

	return msg, data, nil
}

// WriteMessage marshals a proto message and writes it as a framed message
// with the given tag.
func WriteMessage(w io.Writer, tag Tag, msg proto.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return xerrors.Errorf("marshal: %w", err)
	}
	return WriteFrame(w, tag, data)
}
