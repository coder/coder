package drpcsdk

import (
	"runtime/debug"

	"storj.io/drpc"
	"storj.io/drpc/drpcserver"

	"cdr.dev/slog/v3"
)

// NewServer constructs a dRPC server that recovers panics from RPC handlers.
func NewServer(logger slog.Logger, handler drpc.Handler, options drpcserver.Options) *drpcserver.Server {
	return drpcserver.NewWithOptions(&recoverHandler{
		logger:  logger,
		handler: handler,
	}, options)
}

type recoverHandler struct {
	logger  slog.Logger
	handler drpc.Handler
}

func (h *recoverHandler) HandleRPC(stream drpc.Stream, rpc string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			h.logger.Error(stream.Context(),
				"panic serving dRPC request (recovered)",
				slog.F("rpc", rpc),
				slog.F("panic", r),
				slog.F("stack", string(debug.Stack())),
			)
			err = drpc.InternalError.New("panic serving dRPC request")
		}
	}()

	return h.handler.HandleRPC(stream, rpc)
}
