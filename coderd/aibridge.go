package coderd

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/aibridged"
	aibridgedproto "github.com/coder/coder/v2/aibridged/proto"
	"github.com/coder/coder/v2/coderd/util/slice"
)

type rt struct {
	http.RoundTripper

	server *aibridged.Server
}

func (r *rt) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	defer func() {
		fmt.Printf("req to %q started %v completed\n", req.URL.String(), start.Local().Format(time.RFC3339Nano))
	}()

	resp, err := r.RoundTripper.RoundTrip(req)

	return resp, err
}

func (api *API) bridgeAIRequest(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if len(api.AIBridgeDaemons) == 0 {
		http.Error(rw, "no AI bridge daemons running", http.StatusInternalServerError)
		return
	}

	// Random loadbalancing.
	// TODO: introduce better strategy.
	server, err := slice.PickRandom(api.AIBridgeDaemons)
	if err != nil {
		api.Logger.Error(ctx, "failed to pick random AI bridge server", slog.Error(err))
		http.Error(rw, "failed to select AI bridge", http.StatusInternalServerError)
		return
	}

	key, ok := r.Context().Value(aibridged.ContextKeyBridgeAPIKey{}).(string)
	if key == "" || !ok {
		http.Error(rw, "unable to retrieve request session key", http.StatusBadRequest)
		return
	}

	bridge, err := api.createOrLoadBridgeForAPIKey(ctx, key, server.Client)
	if err != nil {
		api.Logger.Error(ctx, "failed to create ai bridge", slog.Error(err))
		http.Error(rw, "failed to create ai bridge", http.StatusInternalServerError)
		return
	}
	http.StripPrefix("/api/v2/aibridge", bridge.Handler()).ServeHTTP(rw, r)
}

func (api *API) createOrLoadBridgeForAPIKey(ctx context.Context, key string, clientFn func() (aibridgedproto.DRPCAIBridgeDaemonClient, bool)) (*aibridged.Bridge, error) {
	if api.AIBridges == nil {
		return nil, xerrors.New("bridge cache storage is not configured")
	}

	api.AIBridgesMu.RLock()
	val, _, ok := api.AIBridges.Get(key)
	api.AIBridgesMu.RUnlock()

	// TODO: TOCTOU potential here
	// TODO: track startup time since it adds latency to first request (histogram count will also help us see how often this occurs)
	if !ok {
		api.AIBridgesMu.Lock()
		defer api.AIBridgesMu.Unlock()

		tools, err := api.fetchTools(ctx, api.Logger, key)
		if err != nil {
			api.Logger.Warn(ctx, "failed to load tools", slog.Error(err))
		}

		bridge, err := aibridged.NewBridge(api.DeploymentValues.AI.BridgeConfig, api.Logger.Named("ai_bridge"), clientFn, tools)
		if err != nil {
			return nil, xerrors.Errorf("create new bridge server: %w", err)
		}

		api.Logger.Info(ctx, "created bridge") // TODO: improve usefulness; log user ID?

		api.AIBridges.Set(key, bridge, time.Minute) // TODO: configurable.
		val = bridge
	}

	return val, nil
}

func (api *API) fetchTools(ctx context.Context, logger slog.Logger, key string) ([]*aibridged.MCPTool, error) {
	url := api.DeploymentValues.AccessURL.String() + "/api/experimental/mcp/http"
	coderMCP, err := aibridged.NewMCPToolBridge("coder", url, map[string]string{
		"Coder-Session-Token": key,
	}, logger.Named("mcp-bridge-coder"))
	if err != nil {
		return nil, xerrors.Errorf("coder MCP bridge setup: %w", err)
	}

	// TODO: add github mcp if external auth is configured.
	var eg errgroup.Group
	eg.Go(func() error {
		ctx, cancel := context.WithTimeout(ctx, time.Second*30)
		defer cancel()

		err := coderMCP.Init(ctx)
		if err == nil {
			return nil
		}
		return xerrors.Errorf("coder: %w", err)
	})

	// This must block requests until MCP proxies are setup.
	if err := eg.Wait(); err != nil {
		return nil, xerrors.Errorf("MCP proxy init: %w", err)
	}

	return coderMCP.ListTools(), nil
}
