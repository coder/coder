package agentssh

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

// streamLocalForwardPayload describes the extra data sent in a
// streamlocal-forward@openssh.com containing the socket path to bind to.
type streamLocalForwardPayload struct {
	SocketPath string
}

// forwardedStreamLocalPayload describes the data sent as the payload in the new
// channel request when a Unix connection is accepted by the listener.
type forwardedStreamLocalPayload struct {
	SocketPath string
	Reserved   uint32
}

// forwardedUnixHandler is a clone of ssh.ForwardedTCPHandler that does
// streamlocal forwarding (aka. unix forwarding) instead of TCP forwarding.
type forwardedUnixHandler struct {
	sync.Mutex
	log                        slog.Logger
	forwards                   map[forwardKey]net.Listener
	blockReversePortForwarding bool

	// socketMu serializes replacing a socket path (unlink and bind) with
	// removing it on close, so a closing listener cannot unlink a socket
	// that another session bound between its ownership check and unlink.
	socketMu sync.Mutex
}

type forwardKey struct {
	sessionID string
	addr      string
}

func newForwardedUnixHandler(log slog.Logger, blockReversePortForwarding bool) *forwardedUnixHandler {
	return &forwardedUnixHandler{
		log:                        log,
		forwards:                   make(map[forwardKey]net.Listener),
		blockReversePortForwarding: blockReversePortForwarding,
	}
}

func (h *forwardedUnixHandler) HandleSSHRequest(ctx ssh.Context, _ *ssh.Server, req *gossh.Request) (bool, []byte) {
	h.log.Debug(ctx, "handling SSH unix forward")
	conn, ok := ctx.Value(ssh.ContextKeyConn).(*gossh.ServerConn)
	if !ok {
		h.log.Warn(ctx, "SSH unix forward request from client with no gossh connection")
		return false, nil
	}
	log := h.log.With(slog.F("session_id", ctx.SessionID()), slog.F("remote_addr", conn.RemoteAddr()))

	switch req.Type {
	case "streamlocal-forward@openssh.com":
		if h.blockReversePortForwarding {
			log.Warn(ctx, "unix reverse port forward blocked")
			return false, nil
		}
		var reqPayload streamLocalForwardPayload
		err := gossh.Unmarshal(req.Payload, &reqPayload)
		if err != nil {
			h.log.Warn(ctx, "parse streamlocal-forward@openssh.com request (SSH unix forward) payload from client", slog.Error(err))
			return false, nil
		}

		addr := reqPayload.SocketPath
		log = log.With(slog.F("socket_path", addr))
		log.Debug(ctx, "request begin SSH unix forward")

		key := forwardKey{
			sessionID: ctx.SessionID(),
			addr:      addr,
		}

		h.Lock()
		_, ok := h.forwards[key]
		h.Unlock()
		if ok {
			// In cases where `ExitOnForwardFailure=yes` is set, returning false
			// here will cause the connection to be closed. To avoid this, and
			// to match OpenSSH behavior, we silently ignore the second forward
			// request.
			log.Warn(ctx, "SSH unix forward request for socket path that is already being forwarded on this session, ignoring")
			return true, nil
		}

		// Create socket parent dir if not exists.
		parentDir := filepath.Dir(addr)
		err = os.MkdirAll(parentDir, 0o700)
		if err != nil {
			log.Warn(ctx, "create parent dir for SSH unix forward request",
				slog.F("parent_dir", parentDir),
				slog.Error(err),
			)
			return false, nil
		}

		ln, err := h.listen(ctx, addr)
		if err != nil {
			log.Warn(ctx, "listen on Unix socket for SSH unix forward request", slog.Error(err))
			return false, nil
		}
		log.Debug(ctx, "SSH unix forward listening on socket")

		// The listener needs to successfully start before it can be added to
		// the map, so we don't have to worry about checking for an existing
		// listener.
		//
		// This is also what the upstream TCP version of this code does.
		h.Lock()
		h.forwards[key] = ln
		h.Unlock()
		log.Debug(ctx, "SSH unix forward added to cache")

		ctx, cancel := context.WithCancel(ctx)
		go func() {
			<-ctx.Done()
			_ = ln.Close()
		}()
		go func() {
			defer cancel()

			for {
				c, err := ln.Accept()
				if err != nil {
					if !xerrors.Is(err, net.ErrClosed) {
						log.Warn(ctx, "accept on local Unix socket for SSH unix forward request", slog.Error(err))
					}
					// closed below
					log.Debug(ctx, "SSH unix forward listener closed")
					break
				}
				log.Debug(ctx, "accepted SSH unix forward connection")
				payload := gossh.Marshal(&forwardedStreamLocalPayload{
					SocketPath: addr,
				})

				go func() {
					ch, reqs, err := conn.OpenChannel("forwarded-streamlocal@openssh.com", payload)
					if err != nil {
						h.log.Warn(ctx, "open SSH unix forward channel to client", slog.Error(err))
						_ = c.Close()
						return
					}
					go gossh.DiscardRequests(reqs)
					Bicopy(ctx, ch, c)
				}()
			}

			h.Lock()
			if ln2, ok := h.forwards[key]; ok && ln2 == ln {
				delete(h.forwards, key)
			}
			h.Unlock()
			log.Debug(ctx, "SSH unix forward listener removed from cache")
			_ = ln.Close()
		}()

		return true, nil

	case "cancel-streamlocal-forward@openssh.com":
		var reqPayload streamLocalForwardPayload
		err := gossh.Unmarshal(req.Payload, &reqPayload)
		if err != nil {
			h.log.Warn(ctx, "parse cancel-streamlocal-forward@openssh.com (SSH unix forward) request payload from client", slog.Error(err))
			return false, nil
		}
		log.Debug(ctx, "request to cancel SSH unix forward", slog.F("socket_path", reqPayload.SocketPath))

		key := forwardKey{
			sessionID: ctx.SessionID(),
			addr:      reqPayload.SocketPath,
		}

		h.Lock()
		ln, ok := h.forwards[key]
		delete(h.forwards, key)
		h.Unlock()
		if !ok {
			log.Warn(ctx, "SSH unix forward not found in cache")
			return true, nil
		}
		_ = ln.Close()
		return true, nil

	default:
		return false, nil
	}
}

// listen replaces any existing file at addr with a Unix socket listener. The
// returned listener only removes the socket file on close if the path still
// refers to the socket it bound, so closing an older session's forward does
// not delete a socket that a newer session has bound at the same path.
func (h *forwardedUnixHandler) listen(ctx context.Context, addr string) (net.Listener, error) {
	h.socketMu.Lock()
	defer h.socketMu.Unlock()

	// Remove existing socket if it exists. We do not use os.Remove() here
	// so that directories are kept. Note that it's possible that we will
	// overwrite a regular file here. Both of these behaviors match OpenSSH,
	// however, which is why we unlink.
	err := unlink(addr)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, xerrors.Errorf("remove existing socket: %w", err)
	}

	lc := &net.ListenConfig{}
	ln, err := lc.Listen(ctx, "unix", addr)
	if err != nil {
		return nil, err
	}
	uln, ok := ln.(*net.UnixListener)
	if !ok {
		return ln, nil
	}
	uln.SetUnlinkOnClose(false)

	// If the socket cannot be stat'd, info stays nil and Close falls back
	// to unlinking the path unconditionally.
	info, err := os.Lstat(addr)
	if err != nil {
		h.log.Debug(ctx, "stat SSH unix forward socket after bind", slog.F("socket_path", addr), slog.Error(err))
		info = nil
	}

	return &unixForwardListener{
		UnixListener: uln,
		log:          h.log.With(slog.F("socket_path", addr)),
		path:         addr,
		info:         info,
		socketMu:     &h.socketMu,
	}, nil
}

// unixForwardListener is a Unix socket listener that removes its socket file
// on close only while the path still refers to the socket it bound.
type unixForwardListener struct {
	*net.UnixListener
	log      slog.Logger
	path     string
	info     fs.FileInfo
	socketMu *sync.Mutex

	closeOnce sync.Once
	closeErr  error
}

func (l *unixForwardListener) Close() error {
	l.closeOnce.Do(func() {
		l.socketMu.Lock()
		defer l.socketMu.Unlock()

		// Unlink before closing so the bound socket still pins its inode,
		// which prevents a newer socket at the same path from reusing it
		// and passing the ownership check.
		if l.ownsPath() {
			if err := unlink(l.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				l.log.Warn(context.Background(), "remove SSH unix forward socket on close", slog.Error(err))
			}
		}
		l.closeErr = l.UnixListener.Close()
	})
	return l.closeErr
}

func (l *unixForwardListener) ownsPath() bool {
	if l.info == nil {
		return true
	}
	info, err := os.Lstat(l.path)
	if err != nil {
		return false
	}
	return os.SameFile(l.info, info)
}

// directStreamLocalPayload describes the extra data sent in a
// direct-streamlocal@openssh.com channel request containing the socket path.
type directStreamLocalPayload struct {
	SocketPath string

	Reserved1 string
	Reserved2 uint32
}

func directStreamLocalHandler(_ *ssh.Server, _ *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
	var reqPayload directStreamLocalPayload
	err := gossh.Unmarshal(newChan.ExtraData(), &reqPayload)
	if err != nil {
		_ = newChan.Reject(gossh.ConnectionFailed, "could not parse direct-streamlocal@openssh.com channel payload")
		return
	}

	var dialer net.Dialer
	dconn, err := dialer.DialContext(ctx, "unix", reqPayload.SocketPath)
	if err != nil {
		_ = newChan.Reject(gossh.ConnectionFailed, fmt.Sprintf("dial unix socket %q: %+v", reqPayload.SocketPath, err.Error()))
		return
	}

	ch, reqs, err := newChan.Accept()
	if err != nil {
		_ = dconn.Close()
		return
	}
	go gossh.DiscardRequests(reqs)

	Bicopy(ctx, ch, dconn)
}

// unlink removes files and unlike os.Remove, directories are kept.
func unlink(path string) error {
	// Ignore EINTR like os.Remove, see ignoringEINTR in os/file_posix.go
	// for more details.
	for {
		err := syscall.Unlink(path)
		if !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}
