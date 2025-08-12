package agentapi

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/immortalstreams"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/websocket"
)

// ImmortalStreamsHandler handles immortal stream requests
type ImmortalStreamsHandler struct {
	logger  slog.Logger
	manager *immortalstreams.Manager
}

// NewImmortalStreamsHandler creates a new immortal streams handler
func NewImmortalStreamsHandler(logger slog.Logger, manager *immortalstreams.Manager) *ImmortalStreamsHandler {
	return &ImmortalStreamsHandler{
		logger:  logger,
		manager: manager,
	}
}

// Routes registers the immortal streams routes
func (h *ImmortalStreamsHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Post("/", h.createStream)
	r.Get("/", h.listStreams)
	r.Route("/{streamID}", func(r chi.Router) {
		r.Use(h.streamMiddleware)
		r.Get("/", h.handleStreamRequest)
		r.Delete("/", h.deleteStream)
	})

	return r
}

// streamMiddleware validates and extracts the stream ID
func (*ImmortalStreamsHandler) streamMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		streamIDStr := chi.URLParam(r, "streamID")
		streamID, err := uuid.Parse(streamIDStr)
		if err != nil {
			httpapi.Write(r.Context(), w, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid stream ID format",
			})
			return
		}

		ctx := context.WithValue(r.Context(), streamIDKey{}, streamID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// createStream creates a new immortal stream
func (h *ImmortalStreamsHandler) createStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req codersdk.CreateImmortalStreamRequest
	if !httpapi.Read(ctx, w, r, &req) {
		return
	}

	stream, err := h.manager.CreateStream(ctx, req.TCPPort)
	if err != nil {
		if strings.Contains(err.Error(), "too many immortal streams") {
			httpapi.Write(ctx, w, http.StatusServiceUnavailable, codersdk.Response{
				Message: "Too many Immortal Streams.",
			})
			return
		}
		if strings.Contains(err.Error(), "the connection was refused") {
			httpapi.Write(ctx, w, http.StatusNotFound, codersdk.Response{
				Message: "The connection was refused.",
			})
			return
		}
		httpapi.InternalServerError(w, err)
		return
	}

	httpapi.Write(ctx, w, http.StatusCreated, stream)
}

// listStreams lists all immortal streams
func (h *ImmortalStreamsHandler) listStreams(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	streams := h.manager.ListStreams()
	httpapi.Write(ctx, w, http.StatusOK, streams)
}

// handleStreamRequest handles GET requests for a specific stream and returns stream info or handles WebSocket upgrades
func (h *ImmortalStreamsHandler) handleStreamRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	streamID := getStreamID(ctx)

	// Check if this is a WebSocket upgrade request by looking for WebSocket headers
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		h.handleUpgrade(w, r)
		return
	}

	// Otherwise, return stream info
	stream, ok := h.manager.GetStream(streamID)
	if !ok {
		httpapi.Write(ctx, w, http.StatusNotFound, codersdk.Response{
			Message: "Stream not found",
		})
		return
	}

	httpapi.Write(ctx, w, http.StatusOK, stream.ToAPI())
}

// deleteStream deletes a stream
func (h *ImmortalStreamsHandler) deleteStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	streamID := getStreamID(ctx)

	err := h.manager.DeleteStream(streamID)
	if err != nil {
		if strings.Contains(err.Error(), "stream not found") {
			httpapi.Write(ctx, w, http.StatusNotFound, codersdk.Response{
				Message: "Stream not found",
			})
			return
		}
		httpapi.InternalServerError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleUpgrade handles WebSocket upgrade for immortal stream connections
func (h *ImmortalStreamsHandler) handleUpgrade(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	streamID := getStreamID(ctx)

	// Get sequence numbers from headers
	readSeqNum, err := parseSequenceNumber(r.Header.Get(codersdk.HeaderImmortalStreamSequenceNum))
	if err != nil {
		httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Invalid sequence number: %v", err),
		})
		return
	}

	// Check if stream exists
	_, ok := h.manager.GetStream(streamID)
	if !ok {
		httpapi.Write(ctx, w, http.StatusNotFound, codersdk.Response{
			Message: "Stream not found",
		})
		return
	}

	// Upgrade to WebSocket
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		h.logger.Error(ctx, "failed to accept websocket", slog.Error(err))
		return
	}
	defer conn.Close(websocket.StatusInternalError, "internal error")

	// BackedPipe handles sequence numbers internally
	// No need to expose them through the API

	// Create a WebSocket adapter
	wsConn := &wsConn{
		conn:   conn,
		logger: h.logger,
	}

	// Handle the reconnection
	// BackedPipe only needs the reader sequence number for replay
	err = h.manager.HandleConnection(streamID, wsConn, readSeqNum)
	if err != nil {
		h.logger.Error(ctx, "failed to handle connection", slog.Error(err))
		conn.Close(websocket.StatusInternalError, err.Error())
		return
	}

	// Keep the connection open until it's closed
	<-ctx.Done()
}

// wsConn adapts a WebSocket connection to io.ReadWriteCloser
type wsConn struct {
	conn   *websocket.Conn
	logger slog.Logger
}

func (c *wsConn) Read(p []byte) (n int, err error) {
	typ, data, err := c.conn.Read(context.Background())
	if err != nil {
		return 0, err
	}
	if typ != websocket.MessageBinary {
		return 0, xerrors.Errorf("unexpected message type: %v", typ)
	}
	n = copy(p, data)
	return n, nil
}

func (c *wsConn) Write(p []byte) (n int, err error) {
	err = c.conn.Write(context.Background(), websocket.MessageBinary, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *wsConn) Close() error {
	return c.conn.Close(websocket.StatusNormalClosure, "")
}

// parseSequenceNumber parses a sequence number from a string
func parseSequenceNumber(s string) (uint64, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.ParseUint(s, 10, 64)
}

// getStreamID gets the stream ID from the context
func getStreamID(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(streamIDKey{}).(uuid.UUID)
	return id
}

type streamIDKey struct{}
