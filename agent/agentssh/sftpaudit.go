package agentssh

import (
	"encoding/binary"
	"io"

	"github.com/gliderlabs/ssh"
)

// SFTP protocol constants (SSH_FILEXFER version 3, the version served by
// github.com/pkg/sftp). Only client-to-server request types the decoder
// cares about are listed; all others are skipped by length.
//
// The wire format is specified in
// https://datatracker.ietf.org/doc/html/draft-ietf-secsh-filexfer-02 and
// the @openssh.com extensions (including the historical SSH_FXP_SYMLINK
// argument reversal) in
// https://github.com/openssh/openssh-portable/blob/master/PROTOCOL.
const (
	sshFxpOpen    = 3
	sshFxpSetstat = 9
	sshFxpRemove  = 13
	// sshFxpMkdir is deliberately not logged: it carries no file
	// contents, and uploads into the directory are logged individually.
	sshFxpMkdir    = 14
	sshFxpRmdir    = 15
	sshFxpRename   = 18
	sshFxpSymlink  = 20
	sshFxpExtended = 200

	// SSH_FXP_OPEN pflags.
	sshFxfRead   = 0x00000001
	sshFxfWrite  = 0x00000002
	sshFxfAppend = 0x00000004
	sshFxfCreat  = 0x00000008
	sshFxfTrunc  = 0x00000010
	sshFxfExcl   = 0x00000020

	// sftpAuditMaxPayload caps the size of a request payload the decoder
	// will buffer for parsing. Interesting requests carry paths and are
	// tiny; anything larger (e.g. SSH_FXP_WRITE data) is skipped without
	// buffering. Must comfortably exceed the longest expected path.
	sftpAuditMaxPayload = 64 * 1024

	// sftpAuditMaxPacket is a sanity bound on the declared packet
	// length. Well-behaved clients send packets no larger than ~256KiB
	// (32KiB data plus overhead is typical). A declared length beyond
	// this indicates a corrupt or desynchronized stream, so the decoder
	// disables itself rather than skipping gigabytes.
	sftpAuditMaxPacket = 4 * 1024 * 1024
)

// sftpAuditSession wraps an SSH session serving SFTP, teeing bytes read
// by the SFTP server (i.e. client-to-server requests) into a passive
// protocol decoder. Writes and closes pass through untouched.
type sftpAuditSession struct {
	ssh.Session
	dec *sftpRequestDecoder
}

func newSFTPAuditSession(session ssh.Session, dec *sftpRequestDecoder) *sftpAuditSession {
	return &sftpAuditSession{Session: session, dec: dec}
}

func (s *sftpAuditSession) Read(p []byte) (int, error) {
	n, err := s.Session.Read(p)
	if n > 0 {
		s.dec.feed(p[:n])
	}
	return n, err
}

var _ io.ReadWriteCloser = &sftpAuditSession{}

// sftpRequestDecoder incrementally decodes the client-to-server SFTP
// packet stream and emits file operations for requests that access or
// mutate files. It is strictly passive and fail-open: it never blocks
// the stream, allocates bounded memory, and permanently disables itself
// if the stream doesn't parse as SFTP.
//
// Packet framing: uint32 length, byte type, payload of length-1 bytes.
type sftpRequestDecoder struct {
	emitter *fileTransferOpEmitter

	buf      []byte
	skip     uint64
	disabled bool
}

func newSFTPRequestDecoder(emitter *fileTransferOpEmitter) *sftpRequestDecoder {
	return &sftpRequestDecoder{emitter: emitter}
}

// feed consumes a chunk of the client-to-server byte stream. It never
// fails; on malformed input the decoder disables itself.
func (d *sftpRequestDecoder) feed(p []byte) {
	if d.disabled {
		return
	}

	for len(p) > 0 {
		// Discard the remainder of a packet we chose not to parse.
		if d.skip > 0 {
			n := min(uint64(len(p)), d.skip)
			d.skip -= n
			p = p[n:]
			continue
		}

		// Accumulate the 5-byte header (length + type).
		if len(d.buf) < 5 {
			need := 5 - len(d.buf)
			n := min(need, len(p))
			d.buf = append(d.buf, p[:n]...)
			p = p[n:]
			if len(d.buf) < 5 {
				return
			}
		}

		length := binary.BigEndian.Uint32(d.buf[:4])
		typ := d.buf[4]
		if length < 1 || length > sftpAuditMaxPacket {
			d.disable()
			return
		}
		payloadLen := uint64(length - 1)

		if !d.interesting(typ) || payloadLen > sftpAuditMaxPayload {
			// Skip the payload without buffering it.
			d.buf = d.buf[:0]
			d.skip = payloadLen
			continue
		}

		// Accumulate the full payload for parsing.
		total := 5 + int(payloadLen)
		need := total - len(d.buf)
		n := min(need, len(p))
		d.buf = append(d.buf, p[:n]...)
		p = p[n:]
		if len(d.buf) < total {
			return
		}

		d.parsePacket(typ, d.buf[5:total])
		d.buf = d.buf[:0]
	}
}

func (d *sftpRequestDecoder) disable() {
	d.disabled = true
	d.buf = nil
}

func (*sftpRequestDecoder) interesting(typ byte) bool {
	switch typ {
	case sshFxpOpen, sshFxpSetstat, sshFxpRemove, sshFxpRmdir, sshFxpRename, sshFxpSymlink, sshFxpExtended:
		return true
	default:
		return false
	}
}

// parsePacket decodes the payload of an interesting request and emits
// the corresponding operation. The payload begins with a uint32 request
// id for all handled types.
func (d *sftpRequestDecoder) parsePacket(typ byte, payload []byte) {
	// Consume the request id.
	if _, payload, ok := sftpReadUint32(payload); ok {
		switch typ {
		case sshFxpOpen:
			path, rest, ok := sftpReadString(payload)
			if !ok {
				return
			}
			pflags, _, ok := sftpReadUint32(rest)
			if !ok {
				return
			}
			action, ok := sftpOpenAction(pflags)
			if !ok {
				return
			}
			d.emitOp(action, path, "")
		case sshFxpSetstat:
			// Path followed by ATTRS. Attribute changes matter for
			// auditing because they include truncation (destroys file
			// contents without a remove or upload event) and timestamp
			// changes (timestomping). The individual attributes are not
			// distinguished.
			if path, _, ok := sftpReadString(payload); ok {
				d.emitOp(FileTransferActionSetattr, path, "")
			}
		case sshFxpRemove:
			if path, _, ok := sftpReadString(payload); ok {
				d.emitOp(FileTransferActionRemove, path, "")
			}
		case sshFxpRmdir:
			if path, _, ok := sftpReadString(payload); ok {
				d.emitOp(FileTransferActionRmdir, path, "")
			}
		case sshFxpRename:
			oldPath, rest, ok := sftpReadString(payload)
			if !ok {
				return
			}
			newPath, _, ok := sftpReadString(rest)
			if !ok {
				return
			}
			d.emitOp(FileTransferActionRename, oldPath, newPath)
		case sshFxpSymlink:
			// Matching pkg/sftp's interpretation: the first string is
			// the symlink target, the second is the link being created
			// (the argument order in the protocol was historically
			// reversed; see sshFxpSymlinkPacket in pkg/sftp).
			targetPath, rest, ok := sftpReadString(payload)
			if !ok {
				return
			}
			linkPath, _, ok := sftpReadString(rest)
			if !ok {
				return
			}
			d.emitOp(FileTransferActionSymlink, linkPath, targetPath)
		case sshFxpExtended:
			name, rest, ok := sftpReadString(payload)
			if !ok {
				return
			}
			var action FileTransferAction
			switch name {
			case "posix-rename@openssh.com":
				action = FileTransferActionRename
			case "hardlink@openssh.com":
				// A hard link adds a new name for existing content, so
				// it can expose a file's data under another path.
				action = FileTransferActionHardlink
			default:
				return
			}
			oldPath, rest, ok := sftpReadString(rest)
			if !ok {
				return
			}
			newPath, _, ok := sftpReadString(rest)
			if !ok {
				return
			}
			d.emitOp(action, oldPath, newPath)
		}
	}
}

func (d *sftpRequestDecoder) emitOp(action FileTransferAction, path, target string) {
	d.emitter.Emit(FileTransferOperation{
		Protocol: FileTransferProtocolSFTP,
		Action:   action,
		Path:     path,
		Target:   target,
	})
}

// sftpOpenAction maps SSH_FXP_OPEN pflags to a transfer action: opening
// for read is a download (workspace to client), opening for write is an
// upload (client to workspace), and opening for both is bidirectional
// (either may have occurred).
func sftpOpenAction(pflags uint32) (FileTransferAction, bool) {
	read := pflags&sshFxfRead != 0
	write := pflags&(sshFxfWrite|sshFxfAppend|sshFxfCreat|sshFxfTrunc|sshFxfExcl) != 0
	switch {
	case read && write:
		return FileTransferActionBidirectional, true
	case write:
		return FileTransferActionUpload, true
	case read:
		return FileTransferActionDownload, true
	default:
		return "", false
	}
}

func sftpReadUint32(b []byte) (uint32, []byte, bool) {
	if len(b) < 4 {
		return 0, nil, false
	}
	return binary.BigEndian.Uint32(b[:4]), b[4:], true
}

func sftpReadString(b []byte) (string, []byte, bool) {
	n, rest, ok := sftpReadUint32(b)
	if !ok || uint64(len(rest)) < uint64(n) {
		return "", nil, false
	}
	return string(rest[:n]), rest[n:], true
}
