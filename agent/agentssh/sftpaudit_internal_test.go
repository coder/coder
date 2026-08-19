package agentssh

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

// buildSFTPPacket constructs a wire-format SFTP packet: uint32 length,
// byte type, payload.
func buildSFTPPacket(typ byte, payload []byte) []byte {
	packet := make([]byte, 0, 5+len(payload))
	packet = binary.BigEndian.AppendUint32(packet, uint32(1+len(payload))) // #nosec G115 - test payloads are small
	packet = append(packet, typ)
	return append(packet, payload...)
}

func sftpString(s string) []byte {
	b := binary.BigEndian.AppendUint32(nil, uint32(len(s))) // #nosec G115 - test strings are small
	return append(b, []byte(s)...)
}

func sftpUint32(v uint32) []byte {
	return binary.BigEndian.AppendUint32(nil, v)
}

func TestSFTPRequestDecoder(t *testing.T) {
	t.Parallel()

	openPacket := func(path string, pflags uint32) []byte {
		payload := sftpUint32(1) // request id
		payload = append(payload, sftpString(path)...)
		payload = append(payload, sftpUint32(pflags)...)
		payload = append(payload, sftpUint32(0)...) // empty ATTRS flags
		return buildSFTPPacket(sshFxpOpen, payload)
	}

	pathPacket := func(typ byte, path string) []byte {
		payload := sftpUint32(2)
		payload = append(payload, sftpString(path)...)
		return buildSFTPPacket(typ, payload)
	}

	twoPathPacket := func(typ byte, a, b string) []byte {
		payload := sftpUint32(3)
		payload = append(payload, sftpString(a)...)
		payload = append(payload, sftpString(b)...)
		return buildSFTPPacket(typ, payload)
	}

	tests := []struct {
		name  string
		input []byte
		want  []FileTransferOperation
	}{
		{
			name:  "OpenRead",
			input: openPacket("/etc/passwd", sshFxfRead),
			want: []FileTransferOperation{{
				Protocol: FileTransferProtocolSFTP,
				Action:   FileTransferActionDownload,
				Path:     "/etc/passwd",
			}},
		},
		{
			name:  "OpenWrite",
			input: openPacket("upload.bin", sshFxfWrite|sshFxfCreat|sshFxfTrunc),
			want: []FileTransferOperation{{
				Protocol: FileTransferProtocolSFTP,
				Action:   FileTransferActionUpload,
				Path:     "upload.bin",
			}},
		},
		{
			name:  "OpenBidirectional",
			input: openPacket("db.sqlite", sshFxfRead|sshFxfWrite),
			want: []FileTransferOperation{{
				Protocol: FileTransferProtocolSFTP,
				Action:   FileTransferActionBidirectional,
				Path:     "db.sqlite",
			}},
		},
		{
			name: "Setstat",
			input: buildSFTPPacket(sshFxpSetstat, func() []byte {
				payload := sftpUint32(6)
				payload = append(payload, sftpString("/tmp/stomped")...)
				payload = append(payload, sftpUint32(0x00000008)...) // ACMODTIME flag
				payload = append(payload, sftpUint32(0)...)          // atime
				payload = append(payload, sftpUint32(0)...)          // mtime
				return payload
			}()),
			want: []FileTransferOperation{{
				Protocol: FileTransferProtocolSFTP,
				Action:   FileTransferActionSetattr,
				Path:     "/tmp/stomped",
			}},
		},
		{
			name: "Hardlink",
			input: buildSFTPPacket(sshFxpExtended, func() []byte {
				payload := sftpUint32(7)
				payload = append(payload, sftpString("hardlink@openssh.com")...)
				payload = append(payload, sftpString("/home/coder/secrets.env")...)
				payload = append(payload, sftpString("/tmp/exposed")...)
				return payload
			}()),
			want: []FileTransferOperation{{
				Protocol: FileTransferProtocolSFTP,
				Action:   FileTransferActionHardlink,
				Path:     "/home/coder/secrets.env",
				Target:   "/tmp/exposed",
			}},
		},
		{
			name:  "Remove",
			input: pathPacket(sshFxpRemove, "/tmp/gone"),
			want: []FileTransferOperation{{
				Protocol: FileTransferProtocolSFTP,
				Action:   FileTransferActionRemove,
				Path:     "/tmp/gone",
			}},
		},
		{
			// Directory creation is deliberately not logged: it carries
			// no file contents, and the uploads into the directory are
			// logged individually.
			name:  "MkdirIgnored",
			input: pathPacket(sshFxpMkdir, "newdir"),
			want:  nil,
		},
		{
			name:  "Rmdir",
			input: pathPacket(sshFxpRmdir, "olddir"),
			want: []FileTransferOperation{{
				Protocol: FileTransferProtocolSFTP,
				Action:   FileTransferActionRmdir,
				Path:     "olddir",
			}},
		},
		{
			name:  "Rename",
			input: twoPathPacket(sshFxpRename, "a.txt", "b.txt"),
			want: []FileTransferOperation{{
				Protocol: FileTransferProtocolSFTP,
				Action:   FileTransferActionRename,
				Path:     "a.txt",
				Target:   "b.txt",
			}},
		},
		{
			name: "Symlink",
			// Wire order per pkg/sftp: target path first, then link path.
			input: twoPathPacket(sshFxpSymlink, "/target", "/link"),
			want: []FileTransferOperation{{
				Protocol: FileTransferProtocolSFTP,
				Action:   FileTransferActionSymlink,
				Path:     "/link",
				Target:   "/target",
			}},
		},
		{
			name: "PosixRename",
			input: buildSFTPPacket(sshFxpExtended, func() []byte {
				payload := sftpUint32(4)
				payload = append(payload, sftpString("posix-rename@openssh.com")...)
				payload = append(payload, sftpString("old")...)
				payload = append(payload, sftpString("new")...)
				return payload
			}()),
			want: []FileTransferOperation{{
				Protocol: FileTransferProtocolSFTP,
				Action:   FileTransferActionRename,
				Path:     "old",
				Target:   "new",
			}},
		},
		{
			name: "SkipsUninterestingPackets",
			input: func() []byte {
				// INIT (type 1), then a large WRITE-like packet (type 6),
				// then an OPEN.
				b := buildSFTPPacket(1, sftpUint32(3))
				b = append(b, buildSFTPPacket(6, make([]byte, 32*1024))...)
				b = append(b, openPacket("after-noise", sshFxfRead)...)
				return b
			}(),
			want: []FileTransferOperation{{
				Protocol: FileTransferProtocolSFTP,
				Action:   FileTransferActionDownload,
				Path:     "after-noise",
			}},
		},
		{
			name:  "GarbageDisables",
			input: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want:  nil,
		},
		{
			name: "TruncatedStringFailsSafely",
			input: buildSFTPPacket(sshFxpRemove, func() []byte {
				payload := sftpUint32(5)
				// Declared string length exceeds the remaining payload.
				payload = append(payload, sftpUint32(1000)...)
				payload = append(payload, []byte("short")...)
				return payload
			}()),
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Feed the whole input at once.
			var got []FileTransferOperation
			dec := newSFTPRequestDecoder(func(op FileTransferOperation) {
				got = append(got, op)
			})
			dec.feed(tt.input)
			require.Equal(t, tt.want, got)

			// Feed byte-by-byte to exercise incremental buffering.
			got = nil
			dec = newSFTPRequestDecoder(func(op FileTransferOperation) {
				got = append(got, op)
			})
			for _, b := range tt.input {
				dec.feed([]byte{b})
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestSFTPRequestDecoderDisabledStaysDisabled(t *testing.T) {
	t.Parallel()

	count := 0
	dec := newSFTPRequestDecoder(func(FileTransferOperation) { count++ })
	// A zero-length packet is invalid and disables the decoder.
	dec.feed([]byte{0x00, 0x00, 0x00, 0x00, 0x00})
	require.True(t, dec.disabled)

	// Even a valid OPEN afterwards must be ignored.
	payload := sftpUint32(1)
	payload = append(payload, sftpString("/etc/passwd")...)
	payload = append(payload, sftpUint32(sshFxfRead)...)
	dec.feed(buildSFTPPacket(sshFxpOpen, payload))
	require.Zero(t, count)
}
