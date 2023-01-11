package agent

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
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
	log      slog.Logger
	forwards map[string]net.Listener
}

func (h *forwardedUnixHandler) HandleSSHRequest(ctx ssh.Context, _ *ssh.Server, req *gossh.Request) (bool, []byte) {
	h.Lock()
	if h.forwards == nil {
		h.forwards = make(map[string]net.Listener)
	}
	h.Unlock()
	conn, ok := ctx.Value(ssh.ContextKeyConn).(*gossh.ServerConn)
	if !ok {
		h.log.Warn(ctx, "SSH unix forward request from client with no gossh connection")
		return false, nil
	}

	switch req.Type {
	case "streamlocal-forward@openssh.com":
		var reqPayload streamLocalForwardPayload
		err := gossh.Unmarshal(req.Payload, &reqPayload)
		if err != nil {
			h.log.Warn(ctx, "parse streamlocal-forward@openssh.com request payload from client", slog.Error(err))
			return false, nil
		}

		addr := reqPayload.SocketPath
		h.Lock()
		_, ok := h.forwards[addr]
		h.Unlock()
		if ok {
			h.log.Warn(ctx, "SSH unix forward request for socket path that is already being forwarded (maybe to another client?)",
				slog.F("socket_path", addr),
			)
			return false, nil
		}

		// Create socket parent dir if not exists.
		parentDir := filepath.Dir(addr)
		err = os.MkdirAll(parentDir, 0700)
		if err != nil {
			h.log.Warn(ctx, "create parent dir for SSH unix forward request",
				slog.F("parent_dir", parentDir),
				slog.F("socket_path", addr),
				slog.Error(err),
			)
			return false, nil
		}

		ln, err := net.Listen("unix", addr)
		if err != nil {
			h.log.Warn(ctx, "listen on Unix socket for SSH unix forward request",
				slog.F("socket_path", addr),
				slog.Error(err),
			)
			return false, nil
		}

		// The listener needs to successfully start before it can be added to
		// the map, so we don't have to worry about checking for an existing
		// listener.
		//
		// This is also what the upstream TCP version of this code does.
		h.Lock()
		h.forwards[addr] = ln
		h.Unlock()

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
						h.log.Warn(ctx, "accept on local Unix socket for SSH unix forward request",
							slog.F("socket_path", addr),
							slog.Error(err),
						)
					}
					// closed below
					break
				}
				payload := gossh.Marshal(&forwardedStreamLocalPayload{
					SocketPath: addr,
				})

				go func() {
					ch, reqs, err := conn.OpenChannel("forwarded-streamlocal@openssh.com", payload)
					if err != nil {
						h.log.Warn(ctx, "open SSH channel to forward Unix connection to client",
							slog.F("socket_path", addr),
							slog.Error(err),
						)
						_ = c.Close()
						return
					}
					go gossh.DiscardRequests(reqs)
					Bicopy(ctx, ch, c)
				}()
			}

			h.Lock()
			ln2, ok := h.forwards[addr]
			if ok && ln2 == ln {
				delete(h.forwards, addr)
			}
			h.Unlock()
			_ = ln.Close()
		}()

		return true, nil

	case "cancel-streamlocal-forward@openssh.com":
		var reqPayload streamLocalForwardPayload
		err := gossh.Unmarshal(req.Payload, &reqPayload)
		if err != nil {
			h.log.Warn(ctx, "parse cancel-streamlocal-forward@openssh.com request payload from client", slog.Error(err))
			return false, nil
		}
		h.Lock()
		ln, ok := h.forwards[reqPayload.SocketPath]
		h.Unlock()
		if ok {
			_ = ln.Close()
		}
		return true, nil

	default:
		return false, nil
	}
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
