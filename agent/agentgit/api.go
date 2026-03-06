package agentgit

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/websocket"
)

// API exposes the git watch HTTP routes for the agent.
type API struct {
	logger    slog.Logger
	opts      []Option
	pathStore *PathStore
}

// NewAPI creates a new git watch API.
func NewAPI(logger slog.Logger, pathStore *PathStore, opts ...Option) *API {
	return &API{
		logger:    logger,
		pathStore: pathStore,
		opts:      opts,
	}
}

// Routes returns the chi router for mounting at /api/v0/git.
func (a *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/watch", a.handleWatch)
	return r
}

func (a *API) handleWatch(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionNoContextTakeover,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to accept WebSocket.",
			Detail:  err.Error(),
		})
		return
	}

	// 4 MiB read limit — subscribe messages with many paths can exceed the
	// default 32 KB limit. Matches the SDK/proxy side.
	conn.SetReadLimit(1 << 22)

	stream := wsjson.NewStream[
		codersdk.WorkspaceAgentGitClientMessage,
		codersdk.WorkspaceAgentGitServerMessage,
	](conn, websocket.MessageText, websocket.MessageText, a.logger)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go httpapi.HeartbeatClose(ctx, a.logger, cancel, conn)

	handler := NewHandler(a.logger, a.opts...)

	// scanAndSend performs a scan and sends results if there are
	// changes.
	scanAndSend := func() {
		msg := handler.Scan(ctx)
		if msg != nil {
			if err := stream.Send(*msg); err != nil {
				a.logger.Debug(ctx, "failed to send changes", slog.Error(err))
				cancel()
			}
		}
	}

	// If a chat_id query parameter is provided and the PathStore is
	// available, subscribe to path updates for this chat.
	chatIDStr := r.URL.Query().Get("chat_id")
	if chatIDStr != "" && a.pathStore != nil {
		chatID, parseErr := uuid.Parse(chatIDStr)
		if parseErr == nil {
			// Load any paths that are already tracked for this chat.
			existingPaths := a.pathStore.GetPaths(chatID)
			if len(existingPaths) > 0 {
				handler.Subscribe(existingPaths)
				handler.RequestScan()
			}
			// Subscribe to future path updates.
			notifyCh, unsubscribe := a.pathStore.Subscribe(chatID)
			defer unsubscribe()

			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case <-notifyCh:
						paths := a.pathStore.GetPaths(chatID)
						handler.Subscribe(paths)
						handler.RequestScan()
					}
				}
			}()
		}
	}

	// Start the main run loop in a goroutine.
	go handler.RunLoop(ctx, scanAndSend)

	// Read client messages.
	updates := stream.Chan()
	for {
		select {
		case <-ctx.Done():
			_ = stream.Close(websocket.StatusGoingAway)
			return
		case msg, ok := <-updates:
			if !ok {
				return
			}

			switch msg.Type {
			case codersdk.WorkspaceAgentGitClientMessageTypeRefresh:
				handler.RequestScan()
			default:
				if err := stream.Send(codersdk.WorkspaceAgentGitServerMessage{
					Type:    codersdk.WorkspaceAgentGitServerMessageTypeError,
					Message: "unknown message type",
				}); err != nil {
					return
				}
			}
		}
	}
}
