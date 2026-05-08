package vncproxy

import (
	"encoding/binary"
	"io"

	"golang.org/x/xerrors"
)

// C2S RFB message types per RFC 6143 §7.5 plus widely-used
// extensions noVNC negotiates by default.
const (
	c2sSetPixelFormat           byte = 0
	c2sSetEncodings             byte = 2
	c2sFramebufferUpdateRequest byte = 3
	c2sKeyEvent                 byte = 4
	c2sPointerEvent             byte = 5
	c2sClientCutText            byte = 6
	c2sEnableContinuousUpdates  byte = 150
	c2sClientFence              byte = 248
	c2sXvpClientMessage         byte = 250
	c2sSetDesktopSize           byte = 251
	c2sQEMUClientMessage        byte = 255
)

// pumpClientToServer runs the steady-state C2S parser. It returns nil
// only if src reached a clean EOF on a message boundary; any other
// outcome is an error.
func pumpClientToServer(sess *session, src io.Reader, dst io.Writer, onDrop func(DropEvent)) error {
	for {
		typeBuf, err := readExact(src, 1)
		if err != nil {
			if isMessageBoundaryEOF(err) {
				return nil
			}
			return xerrors.Errorf("read C2S message type: %w", err)
		}
		mt := typeBuf[0]
		switch mt {
		case c2sSetPixelFormat:
			// 1 type + 3 padding + 16 PIXEL_FORMAT.
			body, err := readExact(src, 19)
			if err != nil {
				return xerrors.Errorf("read SetPixelFormat: %w", err)
			}
			// PIXEL_FORMAT.bits-per-pixel is at offset 3 of the body
			// (after the 3 padding bytes).
			bpp := int(body[3])
			if !validBPP(bpp) {
				return xerrors.Errorf("SetPixelFormat: unsupported bits-per-pixel %d", bpp)
			}
			sess.setBPP(bpp)
			if err := writeAll(dst, typeBuf, body); err != nil {
				return xerrors.Errorf("forward SetPixelFormat: %w", err)
			}

		case c2sSetEncodings:
			// 1 padding + 2 number-of-encodings + 4*N encoding type.
			header, err := readExact(src, 3)
			if err != nil {
				return xerrors.Errorf("read SetEncodings header: %w", err)
			}
			n := int(binary.BigEndian.Uint16(header[1:3]))
			const maxEncodings = 1 << 12
			if n > maxEncodings {
				return xerrors.Errorf("SetEncodings: count %d exceeds cap %d", n, maxEncodings)
			}
			body, err := readExact(src, 4*n)
			if err != nil {
				return xerrors.Errorf("read SetEncodings body: %w", err)
			}
			filtered := filterEncodings(body)
			// Rewrite the count field in the header to the filtered
			// length. We keep the original padding byte intact.
			newCount := uint16(len(filtered) / 4) //nolint:gosec // count is bounded by maxEncodings = 4096 above
			binary.BigEndian.PutUint16(header[1:3], newCount)
			if err := writeAll(dst, typeBuf, header, filtered); err != nil {
				return xerrors.Errorf("forward SetEncodings: %w", err)
			}

		case c2sFramebufferUpdateRequest:
			// 9 bytes: 1 incremental + 2 x + 2 y + 2 w + 2 h.
			body, err := readExact(src, 9)
			if err != nil {
				return xerrors.Errorf("read FramebufferUpdateRequest: %w", err)
			}
			if err := writeAll(dst, typeBuf, body); err != nil {
				return xerrors.Errorf("forward FramebufferUpdateRequest: %w", err)
			}

		case c2sKeyEvent:
			// 7 bytes: 1 down + 2 padding + 4 key.
			body, err := readExact(src, 7)
			if err != nil {
				return xerrors.Errorf("read KeyEvent: %w", err)
			}
			if err := writeAll(dst, typeBuf, body); err != nil {
				return xerrors.Errorf("forward KeyEvent: %w", err)
			}

		case c2sPointerEvent:
			// 5 bytes: 1 button-mask + 2 x + 2 y.
			body, err := readExact(src, 5)
			if err != nil {
				return xerrors.Errorf("read PointerEvent: %w", err)
			}
			if err := writeAll(dst, typeBuf, body); err != nil {
				return xerrors.Errorf("forward PointerEvent: %w", err)
			}

		case c2sClientCutText:
			// 3 padding + 4 length + N text. Drop without
			// forwarding.
			header, err := readExact(src, 7)
			if err != nil {
				return xerrors.Errorf("read ClientCutText header: %w", err)
			}
			payloadLen, err := cutTextPayloadLen(header[3:7])
			if err != nil {
				return xerrors.Errorf("ClientCutText: %w", err)
			}
			if err := drainExact(src, payloadLen); err != nil {
				return xerrors.Errorf("drain ClientCutText payload: %w", err)
			}
			onDrop(DropEvent{
				Direction:   DirClientToServer,
				MessageType: mt,
				PayloadLen:  payloadLen,
			})
			// Do not forward anything to the server.

		case c2sEnableContinuousUpdates:
			// 1 enable + 2 x + 2 y + 2 w + 2 h.
			body, err := readExact(src, 9)
			if err != nil {
				return xerrors.Errorf("read EnableContinuousUpdates: %w", err)
			}
			if err := writeAll(dst, typeBuf, body); err != nil {
				return xerrors.Errorf("forward EnableContinuousUpdates: %w", err)
			}

		case c2sClientFence:
			// 3 padding + 4 flags + 1 length + N payload. Mirrors
			// ServerFence in the S2C parser.
			header, err := readExact(src, 8)
			if err != nil {
				return xerrors.Errorf("read ClientFence header: %w", err)
			}
			payload, err := readExact(src, int(header[7]))
			if err != nil {
				return xerrors.Errorf("read ClientFence payload: %w", err)
			}
			if err := writeAll(dst, typeBuf, header, payload); err != nil {
				return xerrors.Errorf("forward ClientFence: %w", err)
			}

		case c2sXvpClientMessage:
			// 1 padding + 1 version + 1 op.
			body, err := readExact(src, 3)
			if err != nil {
				return xerrors.Errorf("read XvpClientMessage: %w", err)
			}
			if err := writeAll(dst, typeBuf, body); err != nil {
				return xerrors.Errorf("forward XvpClientMessage: %w", err)
			}

		case c2sSetDesktopSize:
			// 1 padding + 2 width + 2 height + 1 number-of-screens
			// + 1 padding + N*16 screen array.
			header, err := readExact(src, 7)
			if err != nil {
				return xerrors.Errorf("read SetDesktopSize header: %w", err)
			}
			screens := int(header[5])
			body, err := readExact(src, screens*16)
			if err != nil {
				return xerrors.Errorf("read SetDesktopSize screens: %w", err)
			}
			if err := writeAll(dst, typeBuf, header, body); err != nil {
				return xerrors.Errorf("forward SetDesktopSize: %w", err)
			}

		case c2sQEMUClientMessage:
			// QEMU subprotocol: 1 sub-type + variable. We need the
			// sub-type to size correctly. The only widely-used one
			// is QEMUExtendedKeyEvent (sub-type 0): 1 sub-type + 2
			// down-flag + 4 keysym + 4 keycode = 11 bytes after the
			// outer type.
			subBuf, err := readExact(src, 1)
			if err != nil {
				return xerrors.Errorf("read QEMU sub-type: %w", err)
			}
			switch subBuf[0] {
			case 0: // QEMUExtendedKeyEvent
				rest, err := readExact(src, 10)
				if err != nil {
					return xerrors.Errorf("read QEMUExtendedKeyEvent: %w", err)
				}
				if err := writeAll(dst, typeBuf, subBuf, rest); err != nil {
					return xerrors.Errorf("forward QEMUExtendedKeyEvent: %w", err)
				}
			default:
				return xerrors.Errorf("unsupported QEMU client message sub-type %d", subBuf[0])
			}

		default:
			return xerrors.Errorf("unsupported C2S message type %d", mt)
		}
	}
}

// writeAll writes one or more byte slices to dst, returning the first
// short-write error. It exists to avoid sprinkling slice
// concatenations through the parser.
func writeAll(dst io.Writer, parts ...[]byte) error {
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		if _, err := dst.Write(p); err != nil {
			return err
		}
	}
	return nil
}
