package aibridged

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/aibridged/proto"
	"github.com/coder/coder/v2/codersdk"
)

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
	cfg codersdk.AIBridgeConfig

	httpSrv  *http.Server
	clientFn func() (proto.DRPCAIBridgeDaemonClient, error)
	logger   slog.Logger

	tools map[string]*MCPTool
}

func NewBridge(cfg codersdk.AIBridgeConfig, logger slog.Logger, clientFn func() (proto.DRPCAIBridgeDaemonClient, error), tools map[string][]*MCPTool) (*Bridge, error) {
	var bridge Bridge

	openAIChatProvider := NewOpenAIChatProvider(cfg.OpenAI.BaseURL.String(), cfg.OpenAI.Key.String())
	anthropicMessagesProvider := NewAnthropicMessagesProvider(cfg.Anthropic.BaseURL.String(), cfg.Anthropic.Key.String())

	drpcClient, err := clientFn()
	if err != nil {
		return nil, xerrors.Errorf("could not acquire coderd client for tracking: %w", err)
	}

	mux := &http.ServeMux{}
	mux.HandleFunc("/v1/chat/completions", handleOpenAIChat(openAIChatProvider, drpcClient, tools, logger.Named("openai")))
	mux.HandleFunc("/v1/messages", handleAnthropicMessages(anthropicMessagesProvider, drpcClient, tools, logger.Named("anthropic")))

	srv := &http.Server{
		Handler: mux,

		// TODO: configurable.
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0, // No write timeout for streaming responses.
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
	}

	bridge.cfg = cfg
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

func handleOpenAIChat(provider *OpenAIChatProvider, drpcClient proto.DRPCAIBridgeDaemonClient, tools map[string][]*MCPTool, logger slog.Logger) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// Read and parse request.
		body, err := io.ReadAll(r.Body)
		if err != nil {
			if isConnectionError(err) {
				logger.Debug(r.Context(), "client disconnected during request body read", slog.Error(err))
				return // Don't send error response if client already disconnected
			}
			logger.Error(r.Context(), "failed to read body", slog.Error(err))
			http.Error(w, "failed to read body", http.StatusInternalServerError)
			return
		}
		req, err := provider.ParseRequest(body)
		if err != nil {
			logger.Error(r.Context(), "failed to parse request", slog.Error(err))
			http.Error(w, "failed to parse request", http.StatusBadRequest)
			return
		}

		// Create a new session.
		var sess Session
		if req.Stream {
			sess = provider.NewStreamingSession(req)
		} else {
			sess = provider.NewBlockingSession(req)
		}

		sessID := sess.Init(logger, provider.baseURL, provider.key, NewDRPCTracker(drpcClient), NewInjectedToolManager(tools))
		logger.Debug(context.Background(), "starting openai session", slog.F("session_id", sessID))

		defer func() {
			if err := sess.Close(); err != nil {
				logger.Warn(context.Background(), "failed to close session", slog.Error(err), slog.F("session_id", sessID), slog.F("kind", fmt.Sprintf("%T", sess)))
			}
		}()

		// Process the request.
		if err := sess.ProcessRequest(w, r); err != nil {
			logger.Error(r.Context(), "session execution failed", slog.Error(err))
		}
	}
}

func handleAnthropicMessages(provider *AnthropicMessagesProvider, drpcClient proto.DRPCAIBridgeDaemonClient, tools map[string][]*MCPTool, logger slog.Logger) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// Read and parse request.
		body, err := io.ReadAll(r.Body)
		if err != nil {
			if isConnectionError(err) {
				logger.Debug(r.Context(), "client disconnected during request body read", slog.Error(err))
				return // Don't send error response if client already disconnected
			}
			logger.Error(r.Context(), "failed to read body", slog.Error(err))
			http.Error(w, "failed to read body", http.StatusInternalServerError)
			return
		}
		req, err := provider.ParseRequest(body)
		if err != nil {
			logger.Error(r.Context(), "failed to parse request", slog.Error(err))
			http.Error(w, "failed to parse request", http.StatusBadRequest)
			return
		}

		// Create a new session.
		var sess Session
		if req.UseStreaming() {
			sess = provider.NewStreamingSession(req)
		} else {
			sess = provider.NewBlockingSession(req)
		}

		sessID := sess.Init(logger, provider.baseURL, provider.key, NewDRPCTracker(drpcClient), NewInjectedToolManager(tools))
		logger.Debug(context.Background(), "starting anthropic messages session", slog.F("session_id", sessID))

		defer func() {
			if err := sess.Close(); err != nil {
				logger.Warn(context.Background(), "failed to close session", slog.Error(err), slog.F("session_id", sessID), slog.F("kind", fmt.Sprintf("%T", sess)))
			}
		}()

		// Process the request.
		if err := sess.ProcessRequest(w, r); err != nil {
			logger.Error(r.Context(), "session execution failed", slog.Error(err))
		}
	}
}
