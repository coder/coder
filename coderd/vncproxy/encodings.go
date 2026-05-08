package vncproxy

import "encoding/binary"

// Encoding constants per RFC 6143 §7.7 plus widely-used extensions.
// Codes are int32 because pseudo-encodings use the high bit (negative
// when interpreted as int32).
const (
	encodingRaw      int32 = 0
	encodingCopyRect int32 = 1
	encodingRRE      int32 = 2
	encodingCoRRE    int32 = 4
	encodingHextile  int32 = 5

	// Pseudo-encodings the parser passes through. These are
	// metadata-only (LastRect, DesktopSize, ExtendedDesktopSize,
	// Cursor, etc). We only need to size their rectangle bodies; see
	// rectangleBodyLength.
	encodingDesktopSize          int32 = -223
	encodingExtendedDesktopSize  int32 = -308
	encodingCursor               int32 = -239
	encodingXCursor              int32 = -240
	encodingDesktopName          int32 = -307
	encodingFence                int32 = -312
	encodingContinuousUpdates    int32 = -313
	encodingLastRect             int32 = -224
	encodingQEMUExtendedKeyEvent int32 = -258
)

// allowedEncodings is the set of encoding codes the rectangle parser
// can size. SetEncodings is rewritten to keep only these codes when
// `clipboard_access = false`. Anything else, including extensions we
// have never heard of (e.g. VMware Cursor 0x574D5664, GII, Pixmap),
// gets dropped from the negotiation so the server cannot transmit
// rectangles whose lengths we cannot compute.
//
// We deliberately do NOT include encodings that depend on a sticky
// zlib stream (Tight, TightPNG, Zlib, ZlibHex, TRLE, ZRLE, JPEG, JRLE)
// because tracking their decompression state is out of scope for the
// proxy. We also exclude the ExtendedClipboard pseudo-encoding because
// allowing it would let the client open a binary clipboard channel
// that bypasses our CutText drop.
var allowedEncodings = map[int32]struct{}{
	encodingRaw:                  {},
	encodingCopyRect:             {},
	encodingRRE:                  {},
	encodingCoRRE:                {},
	encodingHextile:              {},
	encodingDesktopSize:          {},
	encodingExtendedDesktopSize:  {},
	encodingCursor:               {},
	encodingXCursor:              {},
	encodingDesktopName:          {},
	encodingFence:                {},
	encodingContinuousUpdates:    {},
	encodingLastRect:             {},
	encodingQEMUExtendedKeyEvent: {},
}

// filterEncodings consumes the raw 4*N body of a SetEncodings message
// and returns a copy with only the encodings the rectangle parser
// can size. The returned slice may be empty, signaling that the
// server is forced to use the default encoding (Raw).
//
// This is an allow-list rather than a deny-list. Many third-party
// extensions exist (VMware Cursor, GII, Pixmap, ContextInformation,
// etc.) and any encoding the rectangle sizer does not know about
// would desync the parser the first time the server emitted it.
// Allow-list semantics make new noVNC extensions safe by default at
// the cost of forcing the policy author to bump the proxy when they
// want to admit a new encoding.
func filterEncodings(body []byte) []byte {
	if len(body)%4 != 0 {
		// Caller already validated 4*N, but be defensive.
		return body
	}
	out := make([]byte, 0, len(body))
	for i := 0; i+4 <= len(body); i += 4 {
		code := int32(binary.BigEndian.Uint32(body[i : i+4])) //nolint:gosec // RFB encoding type is a signed 32-bit integer per RFC 6143 §7.5.2
		if _, ok := allowedEncodings[code]; !ok {
			continue
		}
		out = append(out, body[i:i+4]...)
	}
	return out
}
