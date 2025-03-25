package agentssh

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/gofrs/flock"
	"github.com/spf13/afero"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

const (
	// X11StartPort is the starting port for X11 forwarding, this is the
	// port used for "DISPLAY=localhost:0".
	X11StartPort = 6000
	// X11DefaultDisplayOffset is the default offset for X11 forwarding.
	X11DefaultDisplayOffset = 10
)

// x11Callback is called when the client requests X11 forwarding.
func (*Server) x11Callback(_ ssh.Context, _ ssh.X11) bool {
	// Always allow.
	return true
}

// x11Handler is called when a session has requested X11 forwarding.
// It listens for X11 connections and forwards them to the client.
func (s *Server) x11Handler(ctx ssh.Context, x11 ssh.X11) (displayNumber int, handled bool) {
	serverConn, valid := ctx.Value(ssh.ContextKeyConn).(*gossh.ServerConn)
	if !valid {
		s.logger.Warn(ctx, "failed to get server connection")
		return -1, false
	}

	hostname, err := os.Hostname()
	if err != nil {
		s.logger.Warn(ctx, "failed to get hostname", slog.Error(err))
		s.metrics.x11HandlerErrors.WithLabelValues("hostname").Add(1)
		return -1, false
	}

	ln, display, err := createX11Listener(ctx, *s.config.X11DisplayOffset)
	if err != nil {
		s.logger.Warn(ctx, "failed to create X11 listener", slog.Error(err))
		s.metrics.x11HandlerErrors.WithLabelValues("listen").Add(1)
		return -1, false
	}
	s.trackListener(ln, true)
	defer func() {
		if !handled {
			s.trackListener(ln, false)
			_ = ln.Close()
		}
	}()

	err = addXauthEntry(ctx, s.fs, hostname, strconv.Itoa(display), x11.AuthProtocol, x11.AuthCookie)
	if err != nil {
		s.logger.Warn(ctx, "failed to add Xauthority entry", slog.Error(err))
		s.metrics.x11HandlerErrors.WithLabelValues("xauthority").Add(1)
		return -1, false
	}

	go func() {
		// Don't leave the listener open after the session is gone.
		<-ctx.Done()
		_ = ln.Close()
	}()

	go func() {
		defer ln.Close()
		defer s.trackListener(ln, false)

		for {
			conn, err := ln.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				s.logger.Warn(ctx, "failed to accept X11 connection", slog.Error(err))
				return
			}
			if x11.SingleConnection {
				s.logger.Debug(ctx, "single connection requested, closing X11 listener")
				_ = ln.Close()
			}

			tcpConn, ok := conn.(*net.TCPConn)
			if !ok {
				s.logger.Warn(ctx, fmt.Sprintf("failed to cast connection to TCPConn. got: %T", conn))
				_ = conn.Close()
				continue
			}
			tcpAddr, ok := tcpConn.LocalAddr().(*net.TCPAddr)
			if !ok {
				s.logger.Warn(ctx, fmt.Sprintf("failed to cast local address to TCPAddr. got: %T", tcpConn.LocalAddr()))
				_ = conn.Close()
				continue
			}

			channel, reqs, err := serverConn.OpenChannel("x11", gossh.Marshal(struct {
				OriginatorAddress string
				OriginatorPort    uint32
			}{
				OriginatorAddress: tcpAddr.IP.String(),
				// #nosec G115 - Safe conversion as TCP port numbers are within uint32 range (0-65535)
				OriginatorPort:    uint32(tcpAddr.Port),
			}))
			if err != nil {
				s.logger.Warn(ctx, "failed to open X11 channel", slog.Error(err))
				_ = conn.Close()
				continue
			}
			go gossh.DiscardRequests(reqs)

			if !s.trackConn(ln, conn, true) {
				s.logger.Warn(ctx, "failed to track X11 connection")
				_ = conn.Close()
				continue
			}
			go func() {
				defer s.trackConn(ln, conn, false)
				Bicopy(ctx, conn, channel)
			}()
		}
	}()

	return display, true
}

// createX11Listener creates a listener for X11 forwarding, it will use
// the next available port starting from X11StartPort and displayOffset.
func createX11Listener(ctx context.Context, displayOffset int) (ln net.Listener, display int, err error) {
	var lc net.ListenConfig
	// Look for an open port to listen on.
	for port := X11StartPort + displayOffset; port < math.MaxUint16; port++ {
		ln, err = lc.Listen(ctx, "tcp", fmt.Sprintf("localhost:%d", port))
		if err == nil {
			display = port - X11StartPort
			return ln, display, nil
		}
	}
	return nil, -1, xerrors.Errorf("failed to find open port for X11 listener: %w", err)
}

// addXauthEntry adds an Xauthority entry to the Xauthority file.
// The Xauthority file is located at ~/.Xauthority.
func addXauthEntry(ctx context.Context, fs afero.Fs, host string, display string, authProtocol string, authCookie string) error {
	// Get the Xauthority file path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return xerrors.Errorf("failed to get user home directory: %w", err)
	}

	xauthPath := filepath.Join(homeDir, ".Xauthority")

	lock := flock.New(xauthPath)
	defer lock.Close()
	ok, err := lock.TryLockContext(ctx, 100*time.Millisecond)
	if !ok {
		return xerrors.Errorf("failed to lock Xauthority file: %w", err)
	}
	if err != nil {
		return xerrors.Errorf("failed to lock Xauthority file: %w", err)
	}

	// Open or create the Xauthority file
	file, err := fs.OpenFile(xauthPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return xerrors.Errorf("failed to open Xauthority file: %w", err)
	}
	defer file.Close()

	// Convert the authCookie from hex string to byte slice
	authCookieBytes, err := hex.DecodeString(authCookie)
	if err != nil {
		return xerrors.Errorf("failed to decode auth cookie: %w", err)
	}

	// Read the Xauthority file and look for an existing entry for the host,
	// display, and auth protocol. If an entry is found, overwrite the auth
	// cookie (if it fits). Otherwise, mark the entry for deletion.
	type deleteEntry struct {
		start, end int
	}
	var deleteEntries []deleteEntry
	pos := 0
	updated := false
	for {
		entry, err := readXauthEntry(file)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return xerrors.Errorf("failed to read Xauthority entry: %w", err)
		}

		nextPos := pos + entry.Len()
		cookieStartPos := nextPos - len(entry.authCookie)

		if entry.family == 0x0100 && entry.address == host && entry.display == display && entry.authProtocol == authProtocol {
			if !updated && len(entry.authCookie) == len(authCookieBytes) {
				// Overwrite the auth cookie
				_, err := file.WriteAt(authCookieBytes, int64(cookieStartPos))
				if err != nil {
					return xerrors.Errorf("failed to write auth cookie: %w", err)
				}
				updated = true
			} else {
				// Mark entry for deletion.
				if len(deleteEntries) > 0 && deleteEntries[len(deleteEntries)-1].end == pos {
					deleteEntries[len(deleteEntries)-1].end = nextPos
				} else {
					deleteEntries = append(deleteEntries, deleteEntry{
						start: pos,
						end:   nextPos,
					})
				}
			}
		}

		pos = nextPos
	}

	// In case the magic cookie changed, or we've previously bloated the
	// Xauthority file, we may have to delete entries.
	if len(deleteEntries) > 0 {
		// Read the entire file into memory. This is not ideal, but it's the
		// simplest way to delete entries from the middle of the file. The
		// Xauthority file is small, so this should be fine.
		_, err = file.Seek(0, io.SeekStart)
		if err != nil {
			return xerrors.Errorf("failed to seek Xauthority file: %w", err)
		}
		data, err := io.ReadAll(file)
		if err != nil {
			return xerrors.Errorf("failed to read Xauthority file: %w", err)
		}

		// Delete the entries in reverse order.
		for i := len(deleteEntries) - 1; i >= 0; i-- {
			entry := deleteEntries[i]
			// Safety check: ensure the entry is still there.
			if entry.start > len(data) || entry.end > len(data) {
				continue
			}
			data = append(data[:entry.start], data[entry.end:]...)
		}

		// Write the data back to the file.
		_, err = file.Seek(0, io.SeekStart)
		if err != nil {
			return xerrors.Errorf("failed to seek Xauthority file: %w", err)
		}
		_, err = file.Write(data)
		if err != nil {
			return xerrors.Errorf("failed to write Xauthority file: %w", err)
		}

		// Truncate the file.
		err = file.Truncate(int64(len(data)))
		if err != nil {
			return xerrors.Errorf("failed to truncate Xauthority file: %w", err)
		}
	}

	// Return if we've already updated the entry.
	if updated {
		return nil
	}

	// Ensure we're at the end (append).
	_, err = file.Seek(0, io.SeekEnd)
	if err != nil {
		return xerrors.Errorf("failed to seek Xauthority file: %w", err)
	}

	// Append Xauthority entry.
	family := uint16(0x0100) // FamilyLocal
	err = binary.Write(file, binary.BigEndian, family)
	if err != nil {
		return xerrors.Errorf("failed to write family: %w", err)
	}

	// #nosec G115 - Safe conversion for host name length which is expected to be within uint16 range
	err = binary.Write(file, binary.BigEndian, uint16(len(host)))
	if err != nil {
		return xerrors.Errorf("failed to write host length: %w", err)
	}
	_, err = file.WriteString(host)
	if err != nil {
		return xerrors.Errorf("failed to write host: %w", err)
	}

	// #nosec G115 - Safe conversion for display name length which is expected to be within uint16 range
	err = binary.Write(file, binary.BigEndian, uint16(len(display)))
	if err != nil {
		return xerrors.Errorf("failed to write display length: %w", err)
	}
	_, err = file.WriteString(display)
	if err != nil {
		return xerrors.Errorf("failed to write display: %w", err)
	}

	err = binary.Write(file, binary.BigEndian, uint16(len(authProtocol)))
	if err != nil {
		return xerrors.Errorf("failed to write auth protocol length: %w", err)
	}
	_, err = file.WriteString(authProtocol)
	if err != nil {
		return xerrors.Errorf("failed to write auth protocol: %w", err)
	}

	err = binary.Write(file, binary.BigEndian, uint16(len(authCookieBytes)))
	if err != nil {
		return xerrors.Errorf("failed to write auth cookie length: %w", err)
	}
	_, err = file.Write(authCookieBytes)
	if err != nil {
		return xerrors.Errorf("failed to write auth cookie: %w", err)
	}

	return nil
}

// xauthEntry is an representation of an Xauthority entry.
//
// The Xauthority file format is as follows:
//
// - 16-bit family
// - 16-bit address length
// - address
// - 16-bit display length
// - display
// - 16-bit auth protocol length
// - auth protocol
// - 16-bit auth cookie length
// - auth cookie
type xauthEntry struct {
	family       uint16
	address      string
	display      string
	authProtocol string
	authCookie   []byte
}

func (e xauthEntry) Len() int {
	// 5 * uint16 = 10 bytes for the family/length fields.
	return 2*5 + len(e.address) + len(e.display) + len(e.authProtocol) + len(e.authCookie)
}

func readXauthEntry(r io.Reader) (xauthEntry, error) {
	var entry xauthEntry

	// Read family
	err := binary.Read(r, binary.BigEndian, &entry.family)
	if err != nil {
		return xauthEntry{}, xerrors.Errorf("failed to read family: %w", err)
	}

	// Read address
	var addressLength uint16
	err = binary.Read(r, binary.BigEndian, &addressLength)
	if err != nil {
		return xauthEntry{}, xerrors.Errorf("failed to read address length: %w", err)
	}

	addressBytes := make([]byte, addressLength)
	_, err = r.Read(addressBytes)
	if err != nil {
		return xauthEntry{}, xerrors.Errorf("failed to read address: %w", err)
	}
	entry.address = string(addressBytes)

	// Read display
	var displayLength uint16
	err = binary.Read(r, binary.BigEndian, &displayLength)
	if err != nil {
		return xauthEntry{}, xerrors.Errorf("failed to read display length: %w", err)
	}

	displayBytes := make([]byte, displayLength)
	_, err = r.Read(displayBytes)
	if err != nil {
		return xauthEntry{}, xerrors.Errorf("failed to read display: %w", err)
	}
	entry.display = string(displayBytes)

	// Read auth protocol
	var authProtocolLength uint16
	err = binary.Read(r, binary.BigEndian, &authProtocolLength)
	if err != nil {
		return xauthEntry{}, xerrors.Errorf("failed to read auth protocol length: %w", err)
	}

	authProtocolBytes := make([]byte, authProtocolLength)
	_, err = r.Read(authProtocolBytes)
	if err != nil {
		return xauthEntry{}, xerrors.Errorf("failed to read auth protocol: %w", err)
	}
	entry.authProtocol = string(authProtocolBytes)

	// Read auth cookie
	var authCookieLength uint16
	err = binary.Read(r, binary.BigEndian, &authCookieLength)
	if err != nil {
		return xauthEntry{}, xerrors.Errorf("failed to read auth cookie length: %w", err)
	}

	entry.authCookie = make([]byte, authCookieLength)
	_, err = r.Read(entry.authCookie)
	if err != nil {
		return xauthEntry{}, xerrors.Errorf("failed to read auth cookie: %w", err)
	}

	return entry, nil
}
