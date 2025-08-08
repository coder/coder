package aibridged

import (
	"net/http"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/aibridged/proto"
)

const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"
)

var providerRoutes = map[string][]string{
	ProviderOpenAI:    {"/v1/chat/completions"},
	ProviderAnthropic: {"/v1/messages"},
}

// Bridge is responsible for proxying requests to upstream AI providers.
//
// Characteristics:
// 1.  Client-side cancel
// 2.  No timeout (SSE)
// 3a. client<->coderd conn established
// 3b. coderd<-> provider conn established
// 4a. requests from client<->coderd must be parsed, augmented, and relayed
// 4b. responses from provider->coderd must be parsed, optionally reflected back to client
// 5.  tool calls may be injected and intercepted, transparently to the client
// 6.  multiple calls can be made to provider while holding client<->coderd conn open
// 7.  client<->coderd conn must ONLY be closed on client-side disconn or coderd<->provider non-recoverable error.
type Bridge struct {
	httpSrv  *http.Server
	clientFn func() (proto.DRPCAIBridgeDaemonClient, error)
	logger   slog.Logger

	tools map[string]*MCPTool
}

func NewBridge(registry ProviderRegistry, logger slog.Logger, clientFn func() (proto.DRPCAIBridgeDaemonClient, error), tools ToolRegistry) (*Bridge, error) {
	drpcClient, err := clientFn()
	if err != nil {
		return nil, xerrors.Errorf("could not acquire coderd client for tracking: %w", err)
	}

	mux := &http.ServeMux{}
	for ident, provider := range registry {
		routes, ok := providerRoutes[ident]
		if !ok {
			// Unknown provider identifier; skip.
			continue
		}
		for _, path := range routes {
			mux.HandleFunc(path, NewSessionProcessor(provider, logger, drpcClient, tools))
		}
	}

	srv := &http.Server{
		Handler: mux,

		// TODO: configurable.
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0, // No write timeout for streaming responses.
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
	}

	var bridge Bridge
	bridge.httpSrv = srv
	bridge.clientFn = clientFn
	bridge.logger = logger

	bridge.tools = make(map[string]*MCPTool, len(tools))
	for _, serverTools := range tools {
		for _, tool := range serverTools {
			bridge.tools[tool.ID] = tool
		}
	}

	return &bridge, nil
}

func (b *Bridge) Handler() http.Handler {
	return b.httpSrv.Handler
}
