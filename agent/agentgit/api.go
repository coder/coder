package agentgit

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

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

// maxShowFileSize is the maximum file size returned by the show
// endpoint. Files larger than this are rejected with 422.
const maxShowFileSize = 512 * 1024 // 512 KB

// Routes returns the chi router for mounting at /api/v0/git.
func (a *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/watch", a.handleWatch)
	r.Get("/show", a.handleShow)
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
			// Subscribe to future path updates BEFORE reading
			// existing paths. This ordering guarantees no
			// notification from AddPaths is lost: any call that
			// lands before Subscribe is picked up by GetPaths
			// below, and any call after Subscribe delivers a
			// notification on the channel.
			notifyCh, unsubscribe := a.pathStore.Subscribe(chatID)
			defer unsubscribe()

			// Load any paths that are already tracked for this chat.
			existingPaths := a.pathStore.GetPaths(chatID)
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

// GitShowResponse is the JSON response for the show endpoint.
type GitShowResponse struct {
	Contents string `json:"contents"`
}

func (a *API) handleShow(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	repoRoot := r.URL.Query().Get("repo_root")
	filePath := r.URL.Query().Get("path")
	ref := r.URL.Query().Get("ref")

	if repoRoot == "" || filePath == "" || ref == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Missing required query parameters.",
			Detail:  "repo_root, path, and ref are required.",
		})
		return
	}

	// Validate that repo_root is a git repository by checking for
	// a .git entry.
	gitPath := filepath.Join(repoRoot, ".git")
	if _, err := os.Stat(gitPath); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Not a git repository.",
			Detail:  repoRoot + " does not contain a .git directory.",
		})
		return
	}

	// Run `git show ref:path` to retrieve the file at the given
	// ref.
	//nolint:gosec // ref and filePath are user-provided but we
	// intentionally pass them to git.
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "show", ref+":"+filePath)
	out, err := cmd.Output()
	if err != nil {
		// git show exits non-zero when the path doesn't exist at
		// the given ref.
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "File not found.",
			Detail:  filePath + " does not exist at ref " + ref + ".",
		})
		return
	}

	// Check if the file is binary by looking for null bytes in
	// the first 8 KB.
	checkLen := min(len(out), 8*1024)
	if bytes.ContainsRune(out[:checkLen], '\x00') {
		httpapi.Write(ctx, rw, http.StatusUnprocessableEntity, codersdk.Response{
			Message: "binary file",
		})
		return
	}

	if len(out) > maxShowFileSize {
		httpapi.Write(ctx, rw, http.StatusUnprocessableEntity, codersdk.Response{
			Message: "file too large",
		})
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(rw).Encode(GitShowResponse{
		Contents: string(out),
	})
}
