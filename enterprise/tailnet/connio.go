package tailnet

import (
	"context"
	"io"
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
	auth         agpl.TunnelAuth
	mu           sync.Mutex
	closed       bool
	disconnected bool

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
	requests <-chan *proto.CoordinateRequest,
	responses chan<- *proto.CoordinateResponse,
	id uuid.UUID,
	name string,
	auth agpl.TunnelAuth,
) *connIO {
	peerCtx, cancel := context.WithCancel(peerCtx)
	now := time.Now().Unix()
	c := &connIO{
		id:        id,
		coordCtx:  coordContext,
		peerCtx:   peerCtx,
		cancel:    cancel,
		logger:    logger.With(slog.F("name", name)),
		requests:  requests,
		responses: responses,
		bindings:  bindings,
		tunnels:   tunnels,
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
		if err := sendCtx(c.coordCtx, c.bindings, b); err != nil {
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
			if err := sendCtx(c.coordCtx, c.tunnels, t); err != nil {
				c.logger.Debug(c.coordCtx, "parent context expired while withdrawing tunnels", slog.Error(err))
			}
		}
	}()
	defer c.Close()
	for {
		req, err := recvCtx(c.peerCtx, c.requests)
		if err != nil {
			if xerrors.Is(err, context.Canceled) ||
				xerrors.Is(err, context.DeadlineExceeded) ||
				xerrors.Is(err, io.EOF) {
				c.logger.Debug(c.coordCtx, "exiting io recvLoop", slog.Error(err))
			} else {
				c.logger.Error(c.coordCtx, "failed to receive request", slog.Error(err))
			}
			return
		}
		if err := c.handleRequest(req); err != nil {
			return
		}
	}
}

var errDisconnect = xerrors.New("graceful disconnect")

func (c *connIO) handleRequest(req *proto.CoordinateRequest) error {
	c.logger.Debug(c.peerCtx, "got request")
	if req.UpdateSelf != nil {
		c.logger.Debug(c.peerCtx, "got node update", slog.F("node", req.UpdateSelf))
		b := binding{
			bKey: bKey(c.UniqueID()),
			node: req.UpdateSelf.Node,
			kind: proto.CoordinateResponse_PeerUpdate_NODE,
		}
		if err := sendCtx(c.coordCtx, c.bindings, b); err != nil {
			c.logger.Debug(c.peerCtx, "failed to send binding", slog.Error(err))
			return err
		}
	}
	if req.AddTunnel != nil {
		c.logger.Debug(c.peerCtx, "got add tunnel", slog.F("tunnel", req.AddTunnel))
		dst, err := uuid.FromBytes(req.AddTunnel.Uuid)
		if err != nil {
			c.logger.Error(c.peerCtx, "unable to convert bytes to UUID", slog.Error(err))
			// this shouldn't happen unless there is a client error.  Close the connection so the client
			// doesn't just happily continue thinking everything is fine.
			return err
		}
		if !c.auth.Authorize(dst) {
			return xerrors.New("unauthorized tunnel")
		}
		t := tunnel{
			tKey: tKey{
				src: c.UniqueID(),
				dst: dst,
			},
			active: true,
		}
		if err := sendCtx(c.coordCtx, c.tunnels, t); err != nil {
			c.logger.Debug(c.peerCtx, "failed to send add tunnel", slog.Error(err))
			return err
		}
	}
	if req.RemoveTunnel != nil {
		c.logger.Debug(c.peerCtx, "got remove tunnel", slog.F("tunnel", req.RemoveTunnel))
		dst, err := uuid.FromBytes(req.RemoveTunnel.Uuid)
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
		if err := sendCtx(c.coordCtx, c.tunnels, t); err != nil {
			c.logger.Debug(c.peerCtx, "failed to send remove tunnel", slog.Error(err))
			return err
		}
	}
	if req.Disconnect != nil {
		c.logger.Debug(c.peerCtx, "graceful disconnect")
		c.disconnected = true
		return errDisconnect
	}
	return nil
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
