package coderd

import (
	"context"
	"errors"
	"io"
	"net/http"

	"golang.org/x/xerrors"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/enterprise/aibridged"
	aibridgedproto "github.com/coder/coder/v2/enterprise/aibridged/proto"
	"github.com/coder/coder/v2/enterprise/aibridgedserver"
)

// RegisterInMemoryAIBridgedHTTPHandler mounts [aibridged.Server]'s HTTP router onto
// [API]'s router, so that requests to aibridged will be relayed from Coder's API server
// to the in-memory aibridged.
func (api *API) RegisterInMemoryAIBridgedHTTPHandler(srv http.Handler) {
	if srv == nil {
		panic("aibridged cannot be nil")
	}

	api.aibridgedHandler = srv
}

// CreateInMemoryAIBridgeServer creates a [aibridged.DRPCServer] and returns a
// [aibridged.DRPCClient] to it, connected over an in-memory transport.
// This server is responsible for all the Coder-specific functionality that aibridged
// requires such as persistence and retrieving configuration.
func (api *API) CreateInMemoryAIBridgeServer(dialCtx context.Context) (client aibridged.DRPCClient, err error) {
	// TODO(dannyk): implement options.
	// TODO(dannyk): implement tracing.
	// TODO(dannyk): implement API versioning.

	clientSession, serverSession := drpcsdk.MemTransportPipe()
	defer func() {
		if err != nil {
			_ = clientSession.Close()
			_ = serverSession.Close()
		}
	}()

	mux := drpcmux.New()
	srv, err := aibridgedserver.NewServer(api.ctx, api.Database, api.Logger.Named("aibridgedserver"),
		api.AccessURL.String(), api.DeploymentValues.AI.BridgeConfig, api.ExternalAuthConfigs, api.AGPL.Experiments)
	if err != nil {
		return nil, err
	}
	err = aibridgedproto.DRPCRegisterRecorder(mux, srv)
	if err != nil {
		return nil, xerrors.Errorf("register recorder service: %w", err)
	}
	err = aibridgedproto.DRPCRegisterMCPConfigurator(mux, srv)
	if err != nil {
		return nil, xerrors.Errorf("register MCP configurator service: %w", err)
	}
	err = aibridgedproto.DRPCRegisterAuthorizer(mux, srv)
	if err != nil {
		return nil, xerrors.Errorf("register key validator service: %w", err)
	}
	server := drpcserver.NewWithOptions(&tracing.DRPCHandler{Handler: mux},
		drpcserver.Options{
			Manager: drpcsdk.DefaultDRPCOptions(nil),
			Log: func(err error) {
				if errors.Is(err, io.EOF) {
					return
				}
				api.Logger.Debug(dialCtx, "aibridged drpc server error", slog.Error(err))
			},
		},
	)
	// in-mem pipes aren't technically "websockets" but they have the same properties as far as the
	// API is concerned: they are long-lived connections that we need to close before completing
	// shutdown of the API.
	api.AGPL.WebsocketWaitMutex.Lock()
	api.AGPL.WebsocketWaitGroup.Add(1)
	api.AGPL.WebsocketWaitMutex.Unlock()
	go func() {
		defer api.AGPL.WebsocketWaitGroup.Done()
		// Here we pass the background context, since we want the server to keep serving until the
		// client hangs up. The aibridged is local, in-mem, so there isn't a danger of losing contact with it and
		// having a dead connection we don't know the status of.
		err := server.Serve(context.Background(), serverSession)
		api.Logger.Info(dialCtx, "aibridge daemon disconnected", slog.Error(err))
		// Close the sessions, so we don't leak goroutines serving them.
		_ = clientSession.Close()
		_ = serverSession.Close()
	}()

	return &aibridged.Client{
		Conn:                      clientSession,
		DRPCRecorderClient:        aibridgedproto.NewDRPCRecorderClient(clientSession),
		DRPCMCPConfiguratorClient: aibridgedproto.NewDRPCMCPConfiguratorClient(clientSession),
		DRPCAuthorizerClient:      aibridgedproto.NewDRPCAuthorizerClient(clientSession),
	}, nil
}
