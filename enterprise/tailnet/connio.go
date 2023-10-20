package tailnet

import (
	"context"
	"encoding/json"
	"io"
	"net"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"cdr.dev/slog"
	agpl "github.com/coder/coder/v2/tailnet"
)

// connIO manages the reading and writing to a connected client or agent.  Agent connIOs have their client field set to
// uuid.Nil.  It reads node updates via its decoder, then pushes them onto the bindings channel.  It receives mappings
// via its updates TrackedConn, which then writes them.
type connIO struct {
	pCtx     context.Context
	ctx      context.Context
	cancel   context.CancelFunc
	logger   slog.Logger
	decoder  *json.Decoder
	updates  *agpl.TrackedConn
	bindings chan<- binding
}

func newConnIO(pCtx context.Context,
	logger slog.Logger,
	bindings chan<- binding,
	conn net.Conn,
	id uuid.UUID,
	name string,
	kind agpl.QueueKind,
) *connIO {
	ctx, cancel := context.WithCancel(pCtx)
	c := &connIO{
		pCtx:     pCtx,
		ctx:      ctx,
		cancel:   cancel,
		logger:   logger,
		decoder:  json.NewDecoder(conn),
		updates:  agpl.NewTrackedConn(ctx, cancel, conn, id, logger, name, 0, kind),
		bindings: bindings,
	}
	go c.recvLoop()
	go c.updates.SendUpdates()
	logger.Info(ctx, "serving connection")
	return c
}

func (c *connIO) recvLoop() {
	defer func() {
		// withdraw bindings when we exit.  We need to use the parent context here, since our own context might be
		// canceled, but we still need to withdraw bindings.
		b := binding{
			bKey: bKey{
				id:   c.UniqueID(),
				kind: c.Kind(),
			},
		}
		if err := sendCtx(c.pCtx, c.bindings, b); err != nil {
			c.logger.Debug(c.ctx, "parent context expired while withdrawing bindings", slog.Error(err))
		}
	}()
	defer c.cancel()
	for {
		var node agpl.Node
		err := c.decoder.Decode(&node)
		if err != nil {
			if xerrors.Is(err, io.EOF) ||
				xerrors.Is(err, io.ErrClosedPipe) ||
				xerrors.Is(err, context.Canceled) ||
				xerrors.Is(err, context.DeadlineExceeded) ||
				websocket.CloseStatus(err) > 0 {
				c.logger.Debug(c.ctx, "exiting recvLoop", slog.Error(err))
			} else {
				c.logger.Error(c.ctx, "failed to decode Node update", slog.Error(err))
			}
			return
		}
		c.logger.Debug(c.ctx, "got node update", slog.F("node", node))
		b := binding{
			bKey: bKey{
				id:   c.UniqueID(),
				kind: c.Kind(),
			},
			node: &node,
		}
		if err := sendCtx(c.ctx, c.bindings, b); err != nil {
			c.logger.Debug(c.ctx, "recvLoop ctx expired", slog.Error(err))
			return
		}
	}
}

func (c *connIO) UniqueID() uuid.UUID {
	return c.updates.UniqueID()
}

func (c *connIO) Kind() agpl.QueueKind {
	return c.updates.Kind()
}

func (c *connIO) Enqueue(n []*agpl.Node) error {
	return c.updates.Enqueue(n)
}

func (c *connIO) Name() string {
	return c.updates.Name()
}

func (c *connIO) Stats() (start int64, lastWrite int64) {
	return c.updates.Stats()
}

func (c *connIO) Overwrites() int64 {
	return c.updates.Overwrites()
}

// CoordinatorClose is used by the coordinator when closing a Queue. It
// should skip removing itself from the coordinator.
func (c *connIO) CoordinatorClose() error {
	c.cancel()
	return c.updates.CoordinatorClose()
}

func (c *connIO) Done() <-chan struct{} {
	return c.ctx.Done()
}

func (c *connIO) Close() error {
	c.cancel()
	return c.updates.Close()
}
