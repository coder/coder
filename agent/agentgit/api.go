package agentgit

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/quartz"
	"github.com/coder/websocket"
)

// API exposes the git watch HTTP routes for the agent.
type API struct {
	logger    slog.Logger
	opts      []Option
	pathStore *PathStore
	wsWatcher *httpapi.WSWatcher
}

// NewAPI creates a new git watch API.
func NewAPI(logger slog.Logger, pathStore *PathStore, opts ...Option) *API {
	return &API{
		logger:    logger,
		pathStore: pathStore,
		opts:      opts,
		wsWatcher: httpapi.NewWSWatcher(quartz.NewReal(), nil),
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

	var watchChatID uuid.UUID
	var hasWatchChatID bool
	if chatIDStr := r.URL.Query().Get("chat_id"); chatIDStr != "" {
		if parsedChatID, parseErr := uuid.Parse(chatIDStr); parseErr == nil {
			watchChatID = parsedChatID
			hasWatchChatID = true

			// Reuse header-derived ancestors only when the query chat
			// matches the header chat. Otherwise the ancestors belong
			// to a different chat and would be misleading in logs.
			var ancestors []uuid.UUID
			if chatContext, ok := agentchat.FromContext(ctx); ok && chatContext.ID == watchChatID {
				ancestors = chatContext.AncestorIDs
			}
			ctx = agentchat.WithContext(ctx, watchChatID, ancestors)
		}
	}
	logger := a.logger.With(agentchat.Fields(ctx)...)

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
	](conn, websocket.MessageText, websocket.MessageText, logger)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ctx = a.wsWatcher.Watch(ctx, logger, conn)
	handler := NewHandler(logger, a.opts...)

	// Scan returns nil only when no roots are subscribed; once any
	// root lands it returns either a delta or a heartbeat message.
	scanAndSend := func() {
		msg := handler.Scan(ctx)
		if msg == nil {
			return
		}
		if err := stream.Send(*msg); err != nil {
			logger.Debug(ctx, "failed to send changes", slog.Error(err))
			cancel()
		}
	}

	// If a chat_id query parameter is provided and the PathStore is
	// available, subscribe to path updates for this chat.
	if hasWatchChatID && a.pathStore != nil {
		// Subscribe to future path updates BEFORE reading
		// existing paths. This ordering guarantees no
		// notification from AddPaths is lost: any call that
		// lands before Subscribe is picked up by GetPaths
		// below, and any call after Subscribe delivers a
		// notification on the channel.
		notifyCh, unsubscribe := a.pathStore.Subscribe(watchChatID)
		defer unsubscribe()

		// Load any paths that are already tracked for this chat.
		existingPaths := a.pathStore.GetPaths(watchChatID)
		if len(existingPaths) > 0 {
			handler.Subscribe(existingPaths)
			handler.RequestScan()
		}

		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-notifyCh:
					paths := a.pathStore.GetPaths(watchChatID)
					handler.Subscribe(paths)
					handler.RequestScan()
				}
			}
		}()
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
