package tailmesh

import (
	"context"

	"cdr.dev/slog"
	"github.com/coder/coder/tailnet"
	"tailscale.com/derp"
	"tailscale.com/derp/derphttp"
)

func New(logger slog.Logger, server *derp.Server) *Mesh {

}

type Mesh struct {
	logger slog.Logger
	server *derp.Server
	ctx    context.Context

	active map[string]context.CancelFunc
}

func (m *Mesh) SetAddresses(addresses []string) {
	for _, address := range addresses {
		client, err := derphttp.NewClient(m.server.PrivateKey(), address, tailnet.Logger(m.logger))
		if err != nil {

		}
		go client.RunWatchConnectionLoop()
	}
}
