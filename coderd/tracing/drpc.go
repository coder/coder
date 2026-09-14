package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"storj.io/drpc"
	"storj.io/drpc/drpcmetadata"
)

type DRPCHandler struct {
	Handler drpc.Handler
}

func (t *DRPCHandler) HandleRPC(stream drpc.Stream, rpc string) error {
	metadata, ok := drpcmetadata.Get(stream.Context())
	if ok {
		ctx := otel.GetTextMapPropagator().Extract(stream.Context(), propagation.MapCarrier(metadata))
		stream = &drpcStreamWrapper{Stream: stream, ctx: ctx}
	}

	return t.Handler.HandleRPC(stream, rpc)
}

type drpcStreamWrapper struct {
	drpc.Stream

	ctx context.Context
}

func (s *drpcStreamWrapper) Context() context.Context { return s.ctx }

type DRPCConn struct {
	drpc.Conn
}

// Invoke implements drpc.Conn's Invoke method with tracing information injected into the context.
func (c *DRPCConn) Invoke(ctx context.Context, rpc string, enc drpc.Encoding, in drpc.Message, out drpc.Message) (err error) {
	return c.Conn.Invoke(c.addMetadata(ctx), rpc, enc, in, out)
}

// NewStream implements drpc.Conn's NewStream method with tracing information injected into the context.
func (c *DRPCConn) NewStream(ctx context.Context, rpc string, enc drpc.Encoding) (_ drpc.Stream, err error) {
	return c.Conn.NewStream(c.addMetadata(ctx), rpc, enc)
}

// addMetadata propagates the headers into a map that we inject into drpc metadata so they are
// sent across the wire for the server to get.
func (*DRPCConn) addMetadata(ctx context.Context) context.Context {
	metadata := make(map[string]string)
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(metadata))
	return drpcmetadata.AddPairs(ctx, metadata)
}
