package agentssh

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/gofrs/flock"
	"github.com/prometheus/client_golang/prometheus"
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
	X11MaxDisplays          = 200
	// X11MaxPort is the highest port we will ever use for X11 forwarding. This limits the total number of TCP sockets
	// we will create. It seems more useful to have a maximum port number than a direct limit on sockets with no max
	// port because we'd like to be able to tell users the exact range of ports the Agent might use.
	X11MaxPort = X11StartPort + X11MaxDisplays
)

// X11Network abstracts the creation of network listeners for X11 forwarding.
// It is intended mainly for testing; production code uses the default
// implementation backed by the operating system networking stack.
type X11Network interface {
	Listen(network, address string) (net.Listener, error)
}

// osNet is the default X11Network implementation that uses the standard
// library network stack.
type osNet struct{}

func (osNet) Listen(network, address string) (net.Listener, error) {
	return net.Listen(network, address)
}

type x11Forwarder struct {
	logger           slog.Logger
	x11HandlerErrors *prometheus.CounterVec
	fs               afero.Fs
	displayOffset    int

	// network creates X11 listener sockets. Defaults to osNet{}.
	network X11Network

	mu          sync.Mutex
	sessions    map[*x11Session]struct{}
	connections map[net.Conn]struct{}
	closing     bool
	wg          sync.WaitGroup
}

type x11Session struct {
	session  ssh.Session
	display  int
	listener net.Listener
	usedAt   time.Time
}

// x11Callback is called when the client requests X11 forwarding.
func (*Server) x11Callback(_ ssh.Context, _ ssh.X11) bool {
	// Always allow.
	return true
}

// x11Handler is called when a session has requested X11 forwarding.
// It listens for X11 connections and forwards them to the client.
func (x *x11Forwarder) x11Handler(sshCtx ssh.Context, sshSession ssh.Session) (displayNumber int, handled bool) {
	x11, hasX11 := sshSession.X11()
	if !hasX11 {
		return -1, false
	}
	serverConn, valid := sshCtx.Value(ssh.ContextKeyConn).(*gossh.ServerConn)
	if !valid {
		x.logger.Warn(sshCtx, "failed to get server connection")
		return -1, false
	}
	ctx := slog.With(sshCtx, slog.F("session_id", fmt.Sprintf("%x", serverConn.SessionID())))

	hostname, err := os.Hostname()
	if err != nil {
		x.logger.Warn(ctx, "failed to get hostname", slog.Error(err))
		x.x11HandlerErrors.WithLabelValues("hostname").Add(1)
		return -1, false
	}

	x11session, err := x.createX11Session(ctx, sshSession)
	if err != nil {
		x.logger.Warn(ctx, "failed to create X11 listener", slog.Error(err))
		x.x11HandlerErrors.WithLabelValues("listen").Add(1)
		return -1, false
	}
	defer func() {
		if !handled {
			x.closeAndRemoveSession(x11session)
		}
	}()

	err = addXauthEntry(ctx, x.fs, hostname, strconv.Itoa(x11session.display), x11.AuthProtocol, x11.AuthCookie)
	if err != nil {
		x.logger.Warn(ctx, "failed to add Xauthority entry", slog.Error(err))
		x.x11HandlerErrors.WithLabelValues("xauthority").Add(1)
		return -1, false
	}

	// clean up the X11 session if the SSH session completes.
	go func() {
		<-ctx.Done()
		x.closeAndRemoveSession(x11session)
	}()

	go x.listenForConnections(ctx, x11session, serverConn, x11)
	x.logger.Debug(ctx, "X11 forwarding started", slog.F("display", x11session.display))

	return x11session.display, true
}

func (x *x11Forwarder) trackGoroutine() (closing bool, done func()) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if !x.closing {
		x.wg.Add(1)
		return false, func() { x.wg.Done() }
	}
	return true, func() {}
}

func (x *x11Forwarder) listenForConnections(
	ctx context.Context, session *x11Session, serverConn *gossh.ServerConn, x11 ssh.X11,
) {
	defer x.closeAndRemoveSession(session)
	if closing, done := x.trackGoroutine(); closing {
		return
	} else { // nolint: revive
		defer done()
	}

	for {
		conn, err := session.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			x.logger.Warn(ctx, "failed to accept X11 connection", slog.Error(err))
			return
		}

		// Update session usage time since a new X11 connection was forwarded.
		x.mu.Lock()
		session.usedAt = time.Now()
		x.mu.Unlock()
		if x11.SingleConnection {
			x.logger.Debug(ctx, "single connection requested, closing X11 listener")
			x.closeAndRemoveSession(session)
		}

		var originAddr string
		var originPort uint32

		if tcpConn, ok := conn.(*net.TCPConn); ok {
			if tcpAddr, ok := tcpConn.LocalAddr().(*net.TCPAddr); ok && tcpAddr != nil {
				originAddr = tcpAddr.IP.String()
				// #nosec G115 - Safe conversion as TCP port numbers are within uint32 range (0-65535)
				originPort = uint32(tcpAddr.Port)
			}
		}
		// Fallback values for in-memory or non-TCP connections.
		if originAddr == "" {
			originAddr = "127.0.0.1"
		}

		channel, reqs, err := serverConn.OpenChannel("x11", gossh.Marshal(struct {
			OriginatorAddress string
			OriginatorPort    uint32
		}{
			OriginatorAddress: originAddr,
			OriginatorPort:    originPort,
		}))
		if err != nil {
			x.logger.Warn(ctx, "failed to open X11 channel", slog.Error(err))
			_ = conn.Close()
			continue
		}
		go gossh.DiscardRequests(reqs)

		if !x.trackConn(conn, true) {
			x.logger.Warn(ctx, "failed to track X11 connection")
			_ = conn.Close()
			continue
		}
		go func() {
			defer x.trackConn(conn, false)
			Bicopy(ctx, conn, channel)
		}()
	}
}

// closeAndRemoveSession closes and removes the session.
func (x *x11Forwarder) closeAndRemoveSession(x11session *x11Session) {
	_ = x11session.listener.Close()
	x.mu.Lock()
	delete(x.sessions, x11session)
	x.mu.Unlock()
}

// createX11Session creates an X11 forwarding session.
func (x *x11Forwarder) createX11Session(ctx context.Context, sshSession ssh.Session) (*x11Session, error) {
	var (
		ln      net.Listener
		display int
		err     error
	)
	// retry listener creation after evictions. Limit to 10 retries to prevent pathological cases looping forever.
	const maxRetries = 10
	for try := range maxRetries {
		ln, display, err = x.createX11Listener(ctx)
		if err == nil {
			break
		}
		if try == maxRetries-1 {
			return nil, xerrors.New("max retries exceeded while creating X11 session")
		}
		x.logger.Warn(ctx, "failed to create X11 listener; will evict an X11 forwarding session",
			slog.F("num_current_sessions", x.numSessions()),
			slog.Error(err))
		x.evictLeastRecentlyUsedSession()
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.closing {
		closeErr := ln.Close()
		if closeErr != nil {
			x.logger.Error(ctx, "error closing X11 listener", slog.Error(closeErr))
		}
		return nil, xerrors.New("server is closing")
	}
	x11Sess := &x11Session{
		session:  sshSession,
		display:  display,
		listener: ln,
		usedAt:   time.Now(),
	}
	x.sessions[x11Sess] = struct{}{}
	return x11Sess, nil
}

func (x *x11Forwarder) numSessions() int {
	x.mu.Lock()
	defer x.mu.Unlock()
	return len(x.sessions)
}

func (x *x11Forwarder) popLeastRecentlyUsedSession() *x11Session {
	x.mu.Lock()
	defer x.mu.Unlock()
	var lru *x11Session
	for s := range x.sessions {
		if lru == nil {
			lru = s
			continue
		}
		if s.usedAt.Before(lru.usedAt) {
			lru = s
			continue
		}
	}
	if lru == nil {
		x.logger.Debug(context.Background(), "tried to pop from empty set of X11 sessions")
		return nil
	}
	delete(x.sessions, lru)
	return lru
}

func (x *x11Forwarder) evictLeastRecentlyUsedSession() {
	lru := x.popLeastRecentlyUsedSession()
	if lru == nil {
		return
	}
	err := lru.listener.Close()
	if err != nil {
		x.logger.Error(context.Background(), "failed to close evicted X11 session listener", slog.Error(err))
	}
	// when we evict, we also want to force the SSH session to be closed as well. This is because we intend to reuse
	// the X11 TCP listener port for a new X11 forwarding session. If we left the SSH session up, then graphical apps
	// started in that session could potentially connect to an unintended X11 Server (i.e. the display on a different
	// computer than the one that started the SSH session). Most likely, this session is a zombie anyway if we've
	// reached the maximum number of X11 forwarding sessions.
	err = lru.session.Close()
	if err != nil {
		x.logger.Error(context.Background(), "failed to close evicted X11 SSH session", slog.Error(err))
	}
}

// createX11Listener creates a listener for X11 forwarding, it will use
// the next available port starting from X11StartPort and displayOffset.
func (x *x11Forwarder) createX11Listener(ctx context.Context) (ln net.Listener, display int, err error) {
	// Look for an open port to listen on.
	for port := X11StartPort + x.displayOffset; port <= X11MaxPort; port++ {
		if ctx.Err() != nil {
			return nil, -1, ctx.Err()
		}

		ln, err = x.network.Listen("tcp", fmt.Sprintf("localhost:%d", port))
		if err == nil {
			display = port - X11StartPort
			return ln, display, nil
		}
	}
	return nil, -1, xerrors.Errorf("failed to find open port for X11 listener: %w", err)
}

// trackConn registers the connection with the x11Forwarder. If the server is
// closed, the connection is not registered and should be closed.
//
//nolint:revive
func (x *x11Forwarder) trackConn(c net.Conn, add bool) (ok bool) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if add {
		if x.closing {
			// Server or listener closed.
			return false
		}
		x.wg.Add(1)
		x.connections[c] = struct{}{}
		return true
	}
	x.wg.Done()
	delete(x.connections, c)
	return true
}

func (x *x11Forwarder) Close() error {
	x.mu.Lock()
	x.closing = true

	for s := range x.sessions {
		sErr := s.listener.Close()
		if sErr != nil {
			x.logger.Debug(context.Background(), "failed to close X11 listener", slog.Error(sErr))
		}
	}
	for c := range x.connections {
		cErr := c.Close()
		if cErr != nil {
			x.logger.Debug(context.Background(), "failed to close X11 connection", slog.Error(cErr))
		}
	}

	x.mu.Unlock()
	x.wg.Wait()
	return nil
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

	// #nosec G115 - Safe conversion for auth protocol length which is expected to be within uint16 range
	err = binary.Write(file, binary.BigEndian, uint16(len(authProtocol)))
	if err != nil {
		return xerrors.Errorf("failed to write auth protocol length: %w", err)
	}
	_, err = file.WriteString(authProtocol)
	if err != nil {
		return xerrors.Errorf("failed to write auth protocol: %w", err)
	}

	// #nosec G115 - Safe conversion for auth cookie length which is expected to be within uint16 range
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
