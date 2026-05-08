package vncproxy

import (
	"encoding/binary"
	"io"

	"golang.org/x/xerrors"
)

// S2C RFB message types per RFC 6143 §7.6.
const (
	s2cFramebufferUpdate      byte = 0
	s2cSetColourMapEntries    byte = 1
	s2cBell                   byte = 2
	s2cServerCutText          byte = 3
	s2cEndOfContinuousUpdates byte = 150
	s2cServerFence            byte = 248
	s2cQEMUServerMessage      byte = 255
)

// pumpServerToClient runs the steady-state S2C parser. It returns nil
// only if src reached a clean EOF on a message boundary; any other
// outcome is an error.
func pumpServerToClient(sess *session, src io.Reader, dst io.Writer, onDrop func(DropEvent)) error {
	for {
		typeBuf, err := readExact(src, 1)
		if err != nil {
			if isMessageBoundaryEOF(err) {
				return nil
			}
			return xerrors.Errorf("read S2C message type: %w", err)
		}
		mt := typeBuf[0]
		switch mt {
		case s2cFramebufferUpdate:
			if err := forwardFramebufferUpdate(sess, src, dst, typeBuf); err != nil {
				return xerrors.Errorf("FramebufferUpdate: %w", err)
			}

		case s2cSetColourMapEntries:
			// 1 padding + 2 first-color + 2 number-of-colors + 6*N.
			header, err := readExact(src, 5)
			if err != nil {
				return xerrors.Errorf("read SetColourMapEntries header: %w", err)
			}
			n := int(binary.BigEndian.Uint16(header[3:5]))
			body, err := readExact(src, 6*n)
			if err != nil {
				return xerrors.Errorf("read SetColourMapEntries body: %w", err)
			}
			if err := writeAll(dst, typeBuf, header, body); err != nil {
				return xerrors.Errorf("forward SetColourMapEntries: %w", err)
			}

		case s2cBell:
			if err := writeAll(dst, typeBuf); err != nil {
				return xerrors.Errorf("forward Bell: %w", err)
			}

		case s2cServerCutText:
			// 3 padding + 4 length + N text. Drop without
			// forwarding.
			header, err := readExact(src, 7)
			if err != nil {
				return xerrors.Errorf("read ServerCutText header: %w", err)
			}
			payloadLen, err := cutTextPayloadLen(header[3:7])
			if err != nil {
				return xerrors.Errorf("ServerCutText: %w", err)
			}
			if err := drainExact(src, payloadLen); err != nil {
				return xerrors.Errorf("drain ServerCutText payload: %w", err)
			}
			onDrop(DropEvent{
				Direction:   DirServerToClient,
				MessageType: mt,
				PayloadLen:  payloadLen,
			})

		case s2cEndOfContinuousUpdates:
			// No payload.
			if err := writeAll(dst, typeBuf); err != nil {
				return xerrors.Errorf("forward EndOfContinuousUpdates: %w", err)
			}

		case s2cServerFence:
			// 3 padding + 4 flags + 1 length + N payload.
			header, err := readExact(src, 8)
			if err != nil {
				return xerrors.Errorf("read ServerFence header: %w", err)
			}
			payload, err := readExact(src, int(header[7]))
			if err != nil {
				return xerrors.Errorf("read ServerFence payload: %w", err)
			}
			if err := writeAll(dst, typeBuf, header, payload); err != nil {
				return xerrors.Errorf("forward ServerFence: %w", err)
			}

		default:
			return xerrors.Errorf("unsupported S2C message type %d", mt)
		}
	}
}

// forwardFramebufferUpdate parses and forwards a single
// FramebufferUpdate message. The wire layout is:
//
//	1 byte type (already consumed by caller)
//	1 byte padding
//	2 bytes number-of-rectangles
//	N rectangles, each 12 bytes header + body sized by encoding
//
// We forward the type byte first, then loop rectangle by rectangle so
// the proxy can stream large updates without buffering them whole.
//
// Per the LastRect pseudo-encoding extension (-224), a server may emit
// rectangle count = 0xFFFF and terminate the update with a LastRect
// rectangle of arbitrary index. Real viewers also honor LastRect when
// it appears before the announced count is exhausted. We do the same:
// the loop short-circuits as soon as forwardRectangle reports it.
func forwardFramebufferUpdate(sess *session, src io.Reader, dst io.Writer, typeBuf []byte) error {
	header, err := readExact(src, 3)
	if err != nil {
		return xerrors.Errorf("read header: %w", err)
	}
	rects := int(binary.BigEndian.Uint16(header[1:3]))
	if err := writeAll(dst, typeBuf, header); err != nil {
		return xerrors.Errorf("forward header: %w", err)
	}
	// 0xFFFF is the LastRect sentinel: the loop keeps going until the
	// server emits a LastRect rectangle. We still cap the iteration at
	// the announced count so a malformed stream cannot pin us.
	for i := 0; i < rects; i++ {
		last, err := forwardRectangle(sess, src, dst)
		if err != nil {
			return xerrors.Errorf("rectangle %d/%d: %w", i+1, rects, err)
		}
		if last {
			return nil
		}
	}
	return nil
}

// forwardRectangle forwards one FramebufferUpdate rectangle, including
// its 12-byte header. The body length depends on the encoding code in
// the header and the negotiated bits-per-pixel. The boolean return
// reports whether this rectangle was a LastRect sentinel; the caller
// uses it to terminate the FramebufferUpdate loop the same way real
// viewers do.
func forwardRectangle(sess *session, src io.Reader, dst io.Writer) (bool, error) {
	hdr, err := readExact(src, 12)
	if err != nil {
		return false, xerrors.Errorf("read rectangle header: %w", err)
	}
	width := int(binary.BigEndian.Uint16(hdr[4:6]))
	height := int(binary.BigEndian.Uint16(hdr[6:8]))
	enc := int32(binary.BigEndian.Uint32(hdr[8:12])) //nolint:gosec // RFB rectangle encoding type is a signed 32-bit integer per RFC 6143 §7.6.1
	if err := writeAll(dst, hdr); err != nil {
		return false, xerrors.Errorf("forward rectangle header: %w", err)
	}
	bytesPerPixel := sess.getBPP() / 8
	if bytesPerPixel == 0 {
		return false, xerrors.New("rectangle: bits-per-pixel not initialized")
	}
	bodyLen, err := rectangleBodyLength(enc, width, height, bytesPerPixel, src, dst)
	if err != nil {
		return false, xerrors.Errorf("size rectangle (encoding %d): %w", enc, err)
	}
	if bodyLen < 0 {
		// Hextile already streamed itself directly; nothing more to
		// forward here.
		return enc == encodingLastRect, nil
	}
	if bodyLen == 0 {
		return enc == encodingLastRect, nil
	}
	if err := forwardExact(src, dst, bodyLen); err != nil {
		return false, xerrors.Errorf("forward rectangle body (encoding %d): %w", enc, err)
	}
	return enc == encodingLastRect, nil
}

// rectangleBodyLength computes the byte length of a rectangle's body
// for an encoding the parser can size statically. Encodings that need
// to inspect their body to advance (Hextile) handle their own
// streaming and return -1 to signal "already forwarded".
//
// src/dst are passed in for the streaming case. They are unused for
// every other encoding.
func rectangleBodyLength(enc int32, width, height, bytesPerPixel int, src io.Reader, dst io.Writer) (int, error) {
	switch enc {
	case encodingRaw:
		// width * height * bytesPerPixel.
		return width * height * bytesPerPixel, nil

	case encodingCopyRect:
		// 4 bytes: 2 src-x + 2 src-y.
		return 4, nil

	case encodingRRE:
		// 4 number-of-subrectangles + 1 background pixel + N subs.
		// Each sub is 1 pixel + 8 bytes (x, y, w, h).
		header, err := readExact(src, 4+bytesPerPixel)
		if err != nil {
			return 0, xerrors.Errorf("read RRE header: %w", err)
		}
		n := binary.BigEndian.Uint32(header[:4])
		const maxSubs = 1 << 24
		if n > maxSubs {
			return 0, xerrors.Errorf("RRE subrectangles %d exceeds cap %d", n, maxSubs)
		}
		if err := writeAll(dst, header); err != nil {
			return 0, xerrors.Errorf("forward RRE header: %w", err)
		}
		// Caller streams the rest.
		body := int(n) * (bytesPerPixel + 8)
		return body, nil

	case encodingCoRRE:
		// Similar to RRE but each sub uses 4 bytes for x,y,w,h
		// (single byte each instead of u16).
		header, err := readExact(src, 4+bytesPerPixel)
		if err != nil {
			return 0, xerrors.Errorf("read CoRRE header: %w", err)
		}
		n := binary.BigEndian.Uint32(header[:4])
		const maxSubs = 1 << 24
		if n > maxSubs {
			return 0, xerrors.Errorf("CoRRE subrectangles %d exceeds cap %d", n, maxSubs)
		}
		if err := writeAll(dst, header); err != nil {
			return 0, xerrors.Errorf("forward CoRRE header: %w", err)
		}
		body := int(n) * (bytesPerPixel + 4)
		return body, nil

	case encodingHextile:
		if err := streamHextile(width, height, bytesPerPixel, src, dst); err != nil {
			return 0, err
		}
		return -1, nil

	// Pseudo-encodings.
	case encodingDesktopSize:
		return 0, nil
	case encodingExtendedDesktopSize:
		// 1 number-of-screens + 1 padding + 2 padding + N*16.
		header, err := readExact(src, 4)
		if err != nil {
			return 0, xerrors.Errorf("read ExtendedDesktopSize header: %w", err)
		}
		if err := writeAll(dst, header); err != nil {
			return 0, xerrors.Errorf("forward ExtendedDesktopSize header: %w", err)
		}
		return int(header[0]) * 16, nil
	case encodingCursor:
		// width*height*bytesPerPixel pixels + ceil(width/8)*height
		// bitmask.
		mask := ((width + 7) / 8) * height
		return width*height*bytesPerPixel + mask, nil
	case encodingXCursor:
		// 6 colors + 2 * ceil(width/8)*height bitmask.
		mask := ((width + 7) / 8) * height
		return 6 + 2*mask, nil
	case encodingDesktopName:
		// 4-byte length + N name.
		lenBuf, err := readExact(src, 4)
		if err != nil {
			return 0, xerrors.Errorf("read DesktopName length: %w", err)
		}
		if err := writeAll(dst, lenBuf); err != nil {
			return 0, xerrors.Errorf("forward DesktopName length: %w", err)
		}
		return int(binary.BigEndian.Uint32(lenBuf)), nil
	case encodingFence, encodingContinuousUpdates:
		return 0, nil
	case encodingLastRect:
		return 0, nil
	case encodingQEMUExtendedKeyEvent:
		return 0, nil
	}

	// Anything we did not whitelist is a sign the SetEncodings rewrite
	// failed or the server is sending an unsolicited encoding. Refuse
	// to guess.
	return 0, xerrors.Errorf("unsupported rectangle encoding %d", enc)
}
