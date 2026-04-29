package nats

import (
	"context"

	natsgo "github.com/nats-io/nats.go"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

// Pubsub is an experimental embedded NATS-backed implementation of
// pubsub.Pubsub. See package doc for status.
type Pubsub struct {
	// TODO: embed *server.Server, *natsgo.Conn, subscription registry,
	// metrics, logger, options, and ownership flags.
	_ struct{}
}

// Compile-time assertion that *Pubsub satisfies the pubsub.Pubsub interface.
var _ pubsub.Pubsub = (*Pubsub)(nil)

// New creates a new embedded NATS Pubsub. The returned *Pubsub owns the
// embedded server and client connection and shuts them down on Close.
func New(ctx context.Context, logger slog.Logger, opts Options) (*Pubsub, error) {
	_ = ctx
	_ = logger
	_ = opts
	// TODO: start embedded server, connect in-process client, build Pubsub.
	return nil, xerrors.New("not implemented")
}

// NewFromConn wraps an externally provided *natsgo.Conn. The returned
// *Pubsub does not own the connection; Close drains only package-owned
// subscriptions and wrapper state.
func NewFromConn(logger slog.Logger, nc *natsgo.Conn) (*Pubsub, error) {
	_ = logger
	_ = nc
	// TODO: build Pubsub around an externally owned connection.
	return nil, xerrors.New("not implemented")
}

// Publish publishes a message under the given legacy event name.
func (*Pubsub) Publish(event string, message []byte) error {
	_ = event
	_ = message
	return xerrors.New("not implemented")
}

// Subscribe subscribes a Listener to the given legacy event name.
func (*Pubsub) Subscribe(event string, listener pubsub.Listener) (cancel func(), err error) {
	_ = event
	_ = listener
	return nil, xerrors.New("not implemented")
}

// SubscribeWithErr subscribes a ListenerWithErr to the given legacy event
// name. The listener also receives error deliveries such as
// pubsub.ErrDroppedMessages.
func (*Pubsub) SubscribeWithErr(event string, listener pubsub.ListenerWithErr) (cancel func(), err error) {
	_ = event
	_ = listener
	return nil, xerrors.New("not implemented")
}

// Close drains and shuts down the Pubsub.
func (*Pubsub) Close() error {
	return xerrors.New("not implemented")
}
