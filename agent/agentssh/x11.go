package agentssh

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
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

// x11Callback is called when the client requests X11 forwarding.
// It adds an Xauthority entry to the Xauthority file.
func (s *Server) x11Callback(ctx ssh.Context, x11 ssh.X11) bool {
	hostname, err := os.Hostname()
	if err != nil {
		s.logger.Warn(ctx, "failed to get hostname", slog.Error(err))
		s.metrics.x11HandlerErrors.WithLabelValues("hostname").Add(1)
		return false
	}

	err = s.fs.MkdirAll(s.x11SocketDir, 0o700)
	if err != nil {
		s.logger.Warn(ctx, "failed to make the x11 socket dir", slog.F("dir", s.x11SocketDir), slog.Error(err))
		s.metrics.x11HandlerErrors.WithLabelValues("socker_dir").Add(1)
		return false
	}

	err = addXauthEntry(ctx, s.fs, hostname, strconv.Itoa(int(x11.ScreenNumber)), x11.AuthProtocol, x11.AuthCookie)
	if err != nil {
		s.logger.Warn(ctx, "failed to add Xauthority entry", slog.Error(err))
		s.metrics.x11HandlerErrors.WithLabelValues("xauthority").Add(1)
		return false
	}
	return true
}

// x11Handler is called when a session has requested X11 forwarding.
// It listens for X11 connections and forwards them to the client.
func (s *Server) x11Handler(ctx ssh.Context, x11 ssh.X11) bool {
	serverConn, valid := ctx.Value(ssh.ContextKeyConn).(*gossh.ServerConn)
	if !valid {
		s.logger.Warn(ctx, "failed to get server connection")
		return false
	}
	// We want to overwrite the socket so that subsequent connections will succeed.
	socketPath := filepath.Join(s.x11SocketDir, fmt.Sprintf("X%d", x11.ScreenNumber))
	err := os.Remove(socketPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		s.logger.Warn(ctx, "failed to remove existing X11 socket", slog.Error(err))
		return false
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		s.logger.Warn(ctx, "failed to listen for X11", slog.Error(err))
		return false
	}
	s.trackListener(listener, true)

	go func() {
		defer listener.Close()
		defer s.trackListener(listener, false)
		handledFirstConnection := false

		for {
			conn, err := listener.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				s.logger.Warn(ctx, "failed to accept X11 connection", slog.Error(err))
				return
			}
			if x11.SingleConnection && handledFirstConnection {
				s.logger.Warn(ctx, "X11 connection rejected because single connection is enabled")
				_ = conn.Close()
				continue
			}
			handledFirstConnection = true

			unixConn, ok := conn.(*net.UnixConn)
			if !ok {
				s.logger.Warn(ctx, fmt.Sprintf("failed to cast connection to UnixConn. got: %T", conn))
				return
			}
			unixAddr, ok := unixConn.LocalAddr().(*net.UnixAddr)
			if !ok {
				s.logger.Warn(ctx, fmt.Sprintf("failed to cast local address to UnixAddr. got: %T", unixConn.LocalAddr()))
				return
			}

			channel, reqs, err := serverConn.OpenChannel("x11", gossh.Marshal(struct {
				OriginatorAddress string
				OriginatorPort    uint32
			}{
				OriginatorAddress: unixAddr.Name,
				OriginatorPort:    0,
			}))
			if err != nil {
				s.logger.Warn(ctx, "failed to open X11 channel", slog.Error(err))
				return
			}
			go gossh.DiscardRequests(reqs)
			go Bicopy(ctx, conn, channel)
		}
	}()
	return true
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
	file, err := fs.OpenFile(xauthPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return xerrors.Errorf("failed to open Xauthority file: %w", err)
	}
	defer file.Close()

	// Convert the authCookie from hex string to byte slice
	authCookieBytes, err := hex.DecodeString(authCookie)
	if err != nil {
		return xerrors.Errorf("failed to decode auth cookie: %w", err)
	}

	// Write Xauthority entry
	family := uint16(0x0100) // FamilyLocal
	err = binary.Write(file, binary.BigEndian, family)
	if err != nil {
		return xerrors.Errorf("failed to write family: %w", err)
	}

	err = binary.Write(file, binary.BigEndian, uint16(len(host)))
	if err != nil {
		return xerrors.Errorf("failed to write host length: %w", err)
	}
	_, err = file.WriteString(host)
	if err != nil {
		return xerrors.Errorf("failed to write host: %w", err)
	}

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
