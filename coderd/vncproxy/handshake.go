package vncproxy

import (
	"encoding/binary"
	"io"

	"golang.org/x/xerrors"
)

// runHandshake observes and forwards the RFB handshake byte-for-byte
// while extracting the bits the message-phase parser needs (negotiated
// protocol version, initial bits-per-pixel). RFB 6143 §7 specifies the
// exact byte sequences; this function follows them.
//
// On success, sess.version and sess.bpp are populated.
func runHandshake(sess *session, client, server io.ReadWriter) error {
	// Step 1: server ProtocolVersion (12 bytes), S2C.
	srvVer, err := readExact(server, 12)
	if err != nil {
		return xerrors.Errorf("read server protocol version: %w", err)
	}
	if _, err := client.Write(srvVer); err != nil {
		return xerrors.Errorf("forward server protocol version: %w", err)
	}
	srvVerN, err := parseProtocolVersion(srvVer)
	if err != nil {
		return xerrors.Errorf("parse server protocol version: %w", err)
	}

	// Step 2: client ProtocolVersion (12 bytes), C2S.
	cliVer, err := readExact(client, 12)
	if err != nil {
		return xerrors.Errorf("read client protocol version: %w", err)
	}
	if _, err := server.Write(cliVer); err != nil {
		return xerrors.Errorf("forward client protocol version: %w", err)
	}
	cliVerN, err := parseProtocolVersion(cliVer)
	if err != nil {
		return xerrors.Errorf("parse client protocol version: %w", err)
	}

	// RFB 6143 §7.1.1: peers use the lower of the two versions.
	sess.version = srvVerN
	if cliVerN < sess.version {
		sess.version = cliVerN
	}

	// Step 3: security handshake. Shape depends on negotiated version.
	if err := runSecurityHandshake(sess, client, server); err != nil {
		return xerrors.Errorf("security: %w", err)
	}

	// Step 4: ClientInit (1 byte shared-flag), C2S.
	clientInit, err := readExact(client, 1)
	if err != nil {
		return xerrors.Errorf("read ClientInit: %w", err)
	}
	if _, err := server.Write(clientInit); err != nil {
		return xerrors.Errorf("forward ClientInit: %w", err)
	}

	// Step 5: ServerInit, S2C.
	//   2 width
	//   2 height
	//  16 PIXEL_FORMAT
	//   4 name-length
	//   N name
	serverInit, err := readExact(server, 24)
	if err != nil {
		return xerrors.Errorf("read ServerInit fixed header: %w", err)
	}
	if _, err := client.Write(serverInit); err != nil {
		return xerrors.Errorf("forward ServerInit fixed header: %w", err)
	}
	// PIXEL_FORMAT is bytes 4..19 of ServerInit. Byte 4 is bits-per-pixel.
	bpp := int(serverInit[4])
	if !validBPP(bpp) {
		return xerrors.Errorf("ServerInit: unsupported bits-per-pixel %d", bpp)
	}
	sess.setBPP(bpp)
	nameLen := binary.BigEndian.Uint32(serverInit[20:24])
	if nameLen > 0 {
		// Cap defensively. RFB doesn't require this, but a 1 MiB desktop
		// name would already be absurd; anything larger is almost
		// certainly a desync.
		const maxNameLen = 1 << 20
		if nameLen > maxNameLen {
			return xerrors.Errorf("ServerInit: name length %d exceeds cap %d", nameLen, maxNameLen)
		}
		name, err := readExact(server, int(nameLen))
		if err != nil {
			return xerrors.Errorf("read ServerInit name: %w", err)
		}
		if _, err := client.Write(name); err != nil {
			return xerrors.Errorf("forward ServerInit name: %w", err)
		}
	}
	return nil
}

// runSecurityHandshake handles the security-types negotiation phase
// described in RFB 6143 §7.1.2. The shape differs between RFB 3.3 and
// RFB 3.7+, and the auth subphase is forwarded byte-for-byte without
// interpretation so we can run on top of any negotiated security type.
func runSecurityHandshake(sess *session, client, server io.ReadWriter) error {
	switch sess.version {
	case rfbVersion33:
		// Server picks a single 4-byte security type.
		typ, err := readExact(server, 4)
		if err != nil {
			return xerrors.Errorf("read RFB 3.3 security type: %w", err)
		}
		if _, err := client.Write(typ); err != nil {
			return xerrors.Errorf("forward RFB 3.3 security type: %w", err)
		}
		secType := binary.BigEndian.Uint32(typ)
		switch secType {
		case 0:
			// Connection failed: server follows up with a reason
			// string (4-byte length + text). Forward and stop.
			return forwardLengthPrefixedString(server, client)
		case 1:
			// None: jump straight to SecurityResult.
		case 2:
			// VNC auth: 16-byte challenge S2C, 16-byte response C2S,
			// then SecurityResult.
			if err := forwardExact(server, client, 16); err != nil {
				return xerrors.Errorf("forward RFB 3.3 VNC challenge: %w", err)
			}
			if err := forwardExact(client, server, 16); err != nil {
				return xerrors.Errorf("forward RFB 3.3 VNC response: %w", err)
			}
		default:
			return xerrors.Errorf("unsupported RFB 3.3 security type %d", secType)
		}
		return forwardSecurityResult(sess, server, client)

	case rfbVersion37, rfbVersion38:
		// Server lists available types: 1 byte count, N type bytes.
		countBuf, err := readExact(server, 1)
		if err != nil {
			return xerrors.Errorf("read security types count: %w", err)
		}
		if _, err := client.Write(countBuf); err != nil {
			return xerrors.Errorf("forward security types count: %w", err)
		}
		count := int(countBuf[0])
		if count == 0 {
			// Connection failed; server sends 4-byte reason length
			// then reason text.
			return forwardLengthPrefixedString(server, client)
		}
		if err := forwardExact(server, client, count); err != nil {
			return xerrors.Errorf("forward security types: %w", err)
		}

		// Client picks one type (1 byte).
		choice, err := readExact(client, 1)
		if err != nil {
			return xerrors.Errorf("read client security choice: %w", err)
		}
		if _, err := server.Write(choice); err != nil {
			return xerrors.Errorf("forward client security choice: %w", err)
		}

		switch choice[0] {
		case 1:
			// None.
		case 2:
			// VNC auth: 16-byte challenge S2C, 16-byte response C2S.
			if err := forwardExact(server, client, 16); err != nil {
				return xerrors.Errorf("forward VNC challenge: %w", err)
			}
			if err := forwardExact(client, server, 16); err != nil {
				return xerrors.Errorf("forward VNC response: %w", err)
			}
		default:
			// Anything else (Tight, RA2, VeNCrypt, ...) carries an
			// arbitrary subprotocol we cannot interpret. Forwarding
			// the rest of the stream byte-for-byte is unsafe because
			// we do not know where the security handshake ends. The
			// safest behavior is to refuse the connection: the
			// portabledesktop stack is configured for None, so this
			// path indicates a misconfiguration we should surface.
			return xerrors.Errorf("unsupported security type %d when clipboard interception is enabled", choice[0])
		}

		// RFB 3.8 always sends SecurityResult; RFB 3.7 only sends it
		// for non-None types.
		if sess.version == rfbVersion38 || choice[0] != 1 {
			return forwardSecurityResult(sess, server, client)
		}
		return nil

	default:
		return xerrors.Errorf("unsupported RFB version code %d", sess.version)
	}
}

// forwardSecurityResult forwards the 4-byte SecurityResult. On failure
// (non-zero) the server may follow up with a reason string on RFB 3.8.
func forwardSecurityResult(sess *session, src, dst io.ReadWriter) error {
	resultBuf, err := readExact(src, 4)
	if err != nil {
		return xerrors.Errorf("read SecurityResult: %w", err)
	}
	if _, err := dst.Write(resultBuf); err != nil {
		return xerrors.Errorf("forward SecurityResult: %w", err)
	}
	if binary.BigEndian.Uint32(resultBuf) != 0 {
		// Auth failed.
		if sess.version == rfbVersion38 {
			// RFB 3.8 sends a reason string after a failure.
			return forwardLengthPrefixedString(src, dst)
		}
		// Pre-3.8 just closes; let the io.Copy detect EOF.
		return io.EOF
	}
	return nil
}

// forwardLengthPrefixedString forwards a 4-byte big-endian length
// followed by that many bytes of ASCII reason text. Used when the RFB
// peer signals connection failure.
func forwardLengthPrefixedString(src, dst io.ReadWriter) error {
	lenBuf, err := readExact(src, 4)
	if err != nil {
		return xerrors.Errorf("read failure-reason length: %w", err)
	}
	if _, err := dst.Write(lenBuf); err != nil {
		return xerrors.Errorf("forward failure-reason length: %w", err)
	}
	n := binary.BigEndian.Uint32(lenBuf)
	const maxReasonLen = 1 << 16
	if n > maxReasonLen {
		return xerrors.Errorf("failure-reason length %d exceeds cap %d", n, maxReasonLen)
	}
	if n > 0 {
		if err := forwardExact(src, dst, int(n)); err != nil {
			return xerrors.Errorf("forward failure-reason: %w", err)
		}
	}
	// Connection ended.
	return io.EOF
}

// parseProtocolVersion parses a 12-byte RFB ProtocolVersion string of
// the form "RFB xxx.yyy\n" and returns one of the rfbVersion constants.
// Anything outside the recognized set (3.3, 3.7, 3.8) is rejected; we do
// not silently accept newer versions because parser correctness depends
// on knowing the security-handshake shape.
func parseProtocolVersion(buf []byte) (rfbVersion, error) {
	if len(buf) != 12 {
		return rfbVersionUnknown, xerrors.Errorf("ProtocolVersion length %d != 12", len(buf))
	}
	// Expected literal prefix "RFB ".
	if string(buf[:4]) != "RFB " {
		return rfbVersionUnknown, xerrors.New("ProtocolVersion missing RFB prefix")
	}
	// Trailing newline at byte 11.
	if buf[11] != '\n' {
		return rfbVersionUnknown, xerrors.New("ProtocolVersion missing trailing newline")
	}
	switch string(buf[4:11]) {
	case "003.003":
		return rfbVersion33, nil
	case "003.007":
		return rfbVersion37, nil
	case "003.008":
		return rfbVersion38, nil
	default:
		return rfbVersionUnknown, xerrors.Errorf("unsupported RFB protocol version %q", string(buf[4:11]))
	}
}

// validBPP reports whether bpp is one of the values RFB pixel formats
// permit. The body sizers divide by 8 unconditionally, so 0 must be
// rejected.
func validBPP(bpp int) bool {
	switch bpp {
	case 8, 16, 32:
		return true
	default:
		return false
	}
}
