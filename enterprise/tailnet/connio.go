package tailnet

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	agpl "github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
)

// connIO manages the reading and writing to a connected peer. It reads requests via its requests
// channel, then pushes them onto the bindings or tunnels channel.  It receives responses via calls
// to Enqueue and pushes them onto the responses channel.
type connIO struct {
	id uuid.UUID
	// coordCtx is the parent context, that is, the context of the Coordinator
	coordCtx context.Context
	// peerCtx is the context of the connection to our peer
	peerCtx      context.Context
	cancel       context.CancelFunc
	logger       slog.Logger
	requests     <-chan *proto.CoordinateRequest
	responses    chan<- *proto.CoordinateResponse
	bindings     chan<- binding
	tunnels      chan<- tunnel
	rfhs         chan<- readyForHandshake
	auth         agpl.CoordinateeAuth
	mu           sync.Mutex
	closed       bool
	disconnected bool
	// latest is the most recent, unfiltered snapshot of the mappings we know about
	latest []mapping

	name       string
	start      int64
	lastWrite  int64
	overwrites int64
}

func newConnIO(coordContext context.Context,
	peerCtx context.Context,
	logger slog.Logger,
	bindings chan<- binding,
	tunnels chan<- tunnel,
	rfhs chan<- readyForHandshake,
	requests <-chan *proto.CoordinateRequest,
	responses chan<- *proto.CoordinateResponse,
	id uuid.UUID,
	name string,
	auth agpl.CoordinateeAuth,
) *connIO {
	peerCtx, cancel := context.WithCancel(peerCtx)
	now := time.Now().Unix()
	c := &connIO{
		id:        id,
		coordCtx:  coordContext,
		peerCtx:   peerCtx,
		cancel:    cancel,
		logger:    logger.With(slog.F("name", name), slog.F("peer_id", id)),
		requests:  requests,
		responses: responses,
		bindings:  bindings,
		tunnels:   tunnels,
		rfhs:      rfhs,
		auth:      auth,
		name:      name,
		start:     now,
		lastWrite: now,
	}
	go c.recvLoop()
	c.logger.Info(coordContext, "serving connection")
	return c
}

func (c *connIO) recvLoop() {
	defer func() {
		// withdraw bindings & tunnels when we exit.  We need to use the coordinator context here, since
		// our own context might be canceled, but we still need to withdraw.
		b := binding{
			bKey: bKey(c.UniqueID()),
			kind: proto.CoordinateResponse_PeerUpdate_LOST,
		}
		if c.disconnected {
			b.kind = proto.CoordinateResponse_PeerUpdate_DISCONNECTED
		}
		if err := agpl.SendCtx(c.coordCtx, c.bindings, b); err != nil {
			c.logger.Debug(c.coordCtx, "parent context expired while withdrawing bindings", slog.Error(err))
		}
		// only remove tunnels on graceful disconnect.  If we remove tunnels for lost peers, then
		// this will look like a disconnect from the peer perspective, since we query for active peers
		// by using the tunnel as a join in the database
		if c.disconnected {
			t := tunnel{
				tKey:   tKey{src: c.UniqueID()},
				active: false,
			}
			if err := agpl.SendCtx(c.coordCtx, c.tunnels, t); err != nil {
				c.logger.Debug(c.coordCtx, "parent context expired while withdrawing tunnels", slog.Error(err))
			}
		}
	}()
	defer c.Close()
	for {
		select {
		case <-c.coordCtx.Done():
			c.logger.Debug(c.coordCtx, "exiting io recvLoop; coordinator exit")
			_ = c.Enqueue(&proto.CoordinateResponse{Error: agpl.CloseErrCoordinatorClose})
			return
		case <-c.peerCtx.Done():
			c.logger.Debug(c.peerCtx, "exiting io recvLoop; peer context canceled")
			return
		case req, ok := <-c.requests:
			if !ok {
				c.logger.Debug(c.peerCtx, "exiting io recvLoop; requests chan closed")
				return
			}
			if err := c.handleRequest(req); err != nil {
				if !xerrors.Is(err, errDisconnect) {
					_ = c.Enqueue(&proto.CoordinateResponse{Error: err.Error()})
				}
				return
			}
		}
	}
}

var errDisconnect = xerrors.New("graceful disconnect")

func (c *connIO) handleRequest(req *proto.CoordinateRequest) error {
	c.logger.Debug(c.peerCtx, "got request")
	err := c.auth.Authorize(c.peerCtx, req)
	if err != nil {
		c.logger.Warn(c.peerCtx, "unauthorized request", slog.Error(err))
		return agpl.AuthorizationError{Wrapped: err}
	}

	if req.UpdateSelf != nil {
		c.logger.Debug(c.peerCtx, "got node update", slog.F("node", req.UpdateSelf))
		b := binding{
			bKey: bKey(c.UniqueID()),
			node: req.UpdateSelf.Node,
			kind: proto.CoordinateResponse_PeerUpdate_NODE,
		}
		if err := agpl.SendCtx(c.coordCtx, c.bindings, b); err != nil {
			c.logger.Debug(c.peerCtx, "failed to send binding", slog.Error(err))
			return err
		}
	}
	if req.AddTunnel != nil {
		c.logger.Debug(c.peerCtx, "got add tunnel", slog.F("tunnel", req.AddTunnel))
		dst, err := uuid.FromBytes(req.AddTunnel.Id)
		if err != nil {
			c.logger.Error(c.peerCtx, "unable to convert bytes to UUID", slog.Error(err))
			// this shouldn't happen unless there is a client error.  Close the connection so the client
			// doesn't just happily continue thinking everything is fine.
			return err
		}
		t := tunnel{
			tKey: tKey{
				src: c.UniqueID(),
				dst: dst,
			},
			active: true,
		}
		if err := agpl.SendCtx(c.coordCtx, c.tunnels, t); err != nil {
			c.logger.Debug(c.peerCtx, "failed to send add tunnel", slog.Error(err))
			return err
		}
	}
	if req.RemoveTunnel != nil {
		c.logger.Debug(c.peerCtx, "got remove tunnel", slog.F("tunnel", req.RemoveTunnel))
		dst, err := uuid.FromBytes(req.RemoveTunnel.Id)
		if err != nil {
			c.logger.Error(c.peerCtx, "unable to convert bytes to UUID", slog.Error(err))
			// this shouldn't happen unless there is a client error.  Close the connection so the client
			// doesn't just happily continue thinking everything is fine.
			return err
		}
		t := tunnel{
			tKey: tKey{
				src: c.UniqueID(),
				dst: dst,
			},
			active: false,
		}
		if err := agpl.SendCtx(c.coordCtx, c.tunnels, t); err != nil {
			c.logger.Debug(c.peerCtx, "failed to send remove tunnel", slog.Error(err))
			return err
		}
	}
	if req.Disconnect != nil {
		c.logger.Debug(c.peerCtx, "graceful disconnect")
		c.disconnected = true
		return errDisconnect
	}
	if req.ReadyForHandshake != nil {
		c.logger.Debug(c.peerCtx, "got ready for handshake ", slog.F("rfh", req.ReadyForHandshake))
		for _, rfh := range req.ReadyForHandshake {
			dst, err := uuid.FromBytes(rfh.Id)
			if err != nil {
				c.logger.Error(c.peerCtx, "unable to convert bytes to UUID", slog.Error(err))
				// this shouldn't happen unless there is a client error.  Close the connection so the client
				// doesn't just happily continue thinking everything is fine.
				return err
			}

			mappings := c.getLatestMapping()
			if !slices.ContainsFunc(mappings, func(mapping mapping) bool {
				return mapping.peer == dst
			}) {
				c.logger.Debug(c.peerCtx, "cannot process ready for handshake, src isn't peered with dst",
					slog.F("dst", dst.String()),
				)
				_ = c.Enqueue(&proto.CoordinateResponse{
					Error: fmt.Sprintf("you do not share a tunnel with %q", dst.String()),
				})
				return nil
			}

			if err := agpl.SendCtx(c.coordCtx, c.rfhs, readyForHandshake{
				src: c.id,
				dst: dst,
			}); err != nil {
				c.logger.Debug(c.peerCtx, "failed to send ready for handshake", slog.Error(err))
				return err
			}
		}
	}
	return nil
}

func (c *connIO) setLatestMapping(latest []mapping) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.latest = latest
}

func (c *connIO) getLatestMapping() []mapping {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.latest
}

func (c *connIO) UniqueID() uuid.UUID {
	return c.id
}

func (c *connIO) Enqueue(resp *proto.CoordinateResponse) error {
	atomic.StoreInt64(&c.lastWrite, time.Now().Unix())
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return xerrors.New("connIO closed")
	}
	select {
	case <-c.peerCtx.Done():
		return c.peerCtx.Err()
	case c.responses <- resp:
		c.logger.Debug(c.peerCtx, "wrote response")
		return nil
	default:
		return agpl.ErrWouldBlock
	}
}

func (c *connIO) Name() string {
	return c.name
}

func (c *connIO) Stats() (start int64, lastWrite int64) {
	return c.start, atomic.LoadInt64(&c.lastWrite)
}

func (c *connIO) Overwrites() int64 {
	return atomic.LoadInt64(&c.overwrites)
}

// CoordinatorClose is used by the coordinator when closing a Queue. It
// should skip removing itself from the coordinator.
func (c *connIO) CoordinatorClose() error {
	return c.Close()
}

func (c *connIO) Done() <-chan struct{} {
	return c.peerCtx.Done()
}

func (c *connIO) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.cancel()
	c.closed = true
	close(c.responses)
	return nil
}
