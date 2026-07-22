package coderd

import (
	"context"
	"errors"
	"io"
	"net/http"

	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"

	"cdr.dev/slog/v3"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/aibridged"
	aibridgedproto "github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/coderd/aibridgedserver"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
)

// AIGatewayHandler returns the in-memory AI Gateway HTTP handler
// set by [API.RegisterInMemoryAIBridgedHTTPHandler], or nil if the daemon
// has not been wired in. Callers must apply their own [http.StripPrefix]
// for the route prefix they are mounting under.
func (api *API) AIGatewayHandler() http.Handler {
	return api.aiGatewayHandler
}

// RegisterInMemoryAIBridgedHTTPHandler mounts [aibridged.Server]'s HTTP router onto
// [API]'s router, so that requests to aibridged will be relayed from Coder's API server
// to the in-memory aibridged.
//
// This also registers an in-process [agplaibridge.TransportFactory] so that
// chatd can route coder-agent LLM traffic through aibridge without crossing
// the HTTP route. No license entitlement gate is applied at the factory layer:
// the entitlement check stays on the HTTP route for external callers, while
// in-process coder-agent traffic is the explicit carve-out.
func (api *API) RegisterInMemoryAIBridgedHTTPHandler(srv http.Handler) {
	if srv == nil {
		panic("aibridged cannot be nil")
	}

	api.aiGatewayHandler = srv

	factory := aibridged.NewTransportFactory(http.StripPrefix(agplaibridge.AIGatewayRootPath, srv))
	var asInterface agplaibridge.TransportFactory = factory
	api.AIBridgeTransportFactory.Store(&asInterface)
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
	srv, err := aibridgedserver.NewServer(api.ctx, aibridgedserver.Options{
		Store:               api.Database,
		Pubsub:              api.Pubsub,
		AISeatTracker:       api.AISeatTracker,
		AccessURL:           api.AccessURL.String(),
		GatewayCfg:          api.DeploymentValues.AI.BridgeConfig,
		ExternalAuthConfigs: api.ExternalAuthConfigs,
		Experiments:         api.Experiments,
		Logger:              api.Logger.Named("aibridgedserver"),
		Clock:               api.Clock,
	})
	if err != nil {
		return nil, err
	}
	if err := aibridgedserver.Register(mux, srv); err != nil {
		return nil, err
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
	api.WebsocketWaitMutex.Lock()
	api.WebsocketWaitGroup.Add(1)
	api.WebsocketWaitMutex.Unlock()
	go func() {
		defer api.WebsocketWaitGroup.Done()
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
		Conn:                           clientSession,
		DRPCRecorderClient:             aibridgedproto.NewDRPCRecorderClient(clientSession),
		DRPCMCPConfiguratorClient:      aibridgedproto.NewDRPCMCPConfiguratorClient(clientSession),
		DRPCAuthorizerClient:           aibridgedproto.NewDRPCAuthorizerClient(clientSession),
		DRPCProviderConfiguratorClient: aibridgedproto.NewDRPCProviderConfiguratorClient(clientSession),
	}, nil
}
