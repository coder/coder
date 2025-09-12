package immortalstreams

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/websocket"
)

// Handler handles immortal stream requests
type Handler struct {
	logger  slog.Logger
	manager *Manager
}

// NewHandler creates a new immortal streams handler
func NewHandler(logger slog.Logger, manager *Manager) *Handler {
	return &Handler{
		logger:  logger,
		manager: manager,
	}
}

// Routes registers the immortal streams routes
func (h *Handler) Routes() chi.Router {
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
func (*Handler) streamMiddleware(next http.Handler) http.Handler {
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
func (h *Handler) createStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req codersdk.CreateImmortalStreamRequest
	if !httpapi.Read(ctx, w, r, &req) {
		return
	}

	stream, err := h.manager.CreateStream(ctx, req.TCPPort)
	if err != nil {
		switch {
		case errors.Is(err, ErrTooManyStreams):
			httpapi.Write(ctx, w, http.StatusServiceUnavailable, codersdk.Response{
				Message: "Too many Immortal Streams.",
			})
			return
		case errors.Is(err, ErrConnRefused):
			httpapi.Write(ctx, w, http.StatusNotFound, codersdk.Response{
				Message: "The connection was refused.",
			})
			return
		default:
			httpapi.InternalServerError(w, err)
			return
		}
	}

	httpapi.Write(ctx, w, http.StatusCreated, stream)
}

// listStreams lists all immortal streams
func (h *Handler) listStreams(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	streams := h.manager.ListStreams()
	httpapi.Write(ctx, w, http.StatusOK, streams)
}

// handleStreamRequest handles GET requests for a specific stream and returns stream info or handles WebSocket upgrades
func (h *Handler) handleStreamRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = getStreamID(ctx)

	// Require WebSocket upgrade for connection/reconnect
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		h.handleUpgrade(w, r)
		return
	}

	// Otherwise, return bad request since only reconnect is supported
	httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
		Message: "Upgrade required for immortal stream",
	})
}

// deleteStream deletes a stream
func (h *Handler) deleteStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	streamID := getStreamID(ctx)

	err := h.manager.DeleteStream(streamID)
	if err != nil {
		switch {
		case errors.Is(err, ErrStreamNotFound):
			httpapi.Write(ctx, w, http.StatusNotFound, codersdk.Response{
				Message: "Stream not found",
			})
			return
		default:
			httpapi.InternalServerError(w, err)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleUpgrade handles WebSocket upgrade for immortal stream connections
func (h *Handler) handleUpgrade(w http.ResponseWriter, r *http.Request) {
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
	stream, ok := h.manager.GetStream(streamID)
	if !ok {
		httpapi.Write(ctx, w, http.StatusNotFound, codersdk.Response{
			Message: "Stream not found",
		})
		return
	}

	// Send the current remote reader sequence number as a response header
	// so clients using a backed pipe can resume their writer accurately.
	if stream != nil && stream.GetPipe() != nil {
		readerSeqNum := stream.GetPipe().ReaderSequenceNum()
		w.Header().Set(codersdk.HeaderImmortalStreamSequenceNum, strconv.FormatUint(readerSeqNum, 10))
	}

	// Upgrade to WebSocket
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		h.logger.Error(ctx, "failed to accept websocket", slog.Error(err))
		return
	}

	// Create a context that we can cancel to clean up the connection
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Keep the WebSocket connection alive with periodic pings
	go httpapi.Heartbeat(connCtx, conn)

	// Ensure WebSocket is closed when this function returns
	defer func() {
		conn.Close(websocket.StatusNormalClosure, "connection closed")
	}()

	// Create a WebSocket adapter
	wsConn := &wsConn{
		conn:   conn,
		logger: h.logger,
		ctx:    connCtx,
		cancel: cancel,
	}

	// Handle the reconnection - this establishes the connection
	// BackedPipe only needs the reader sequence number for replay
	err = h.manager.HandleConnection(streamID, wsConn, readSeqNum)
	if err != nil {
		switch {
		case errors.Is(err, ErrStreamNotFound):
			conn.Close(websocket.StatusUnsupportedData, "Stream not found")
			return
		case errors.Is(err, ErrAlreadyConnected):
			conn.Close(websocket.StatusPolicyViolation, "Already connected")
			return
		default:
			h.logger.Error(ctx, "failed to handle connection", slog.Error(err))
			conn.Close(websocket.StatusInternalError, err.Error())
			return
		}
	}

	// Keep the connection open until the context is canceled
	// The wsConn will handle connection closure through its Read/Write methods
	// When the connection is closed, the backing pipe will detect it and the context should be canceled
	<-connCtx.Done()
}

type wsConn struct {
	conn   *websocket.Conn
	logger slog.Logger
	ctx    context.Context
	cancel context.CancelFunc
}

func (c *wsConn) Read(p []byte) (n int, err error) {
	typ, data, err := c.conn.Read(c.ctx)
	if err != nil {
		// Cancel the context when read fails (connection closed)
		c.cancel()
		return 0, err
	}
	if typ != websocket.MessageBinary {
		return 0, xerrors.Errorf("unexpected message type: %v", typ)
	}
	n = copy(p, data)
	return n, nil
}

func (c *wsConn) Write(p []byte) (n int, err error) {
	err = c.conn.Write(c.ctx, websocket.MessageBinary, p)
	if err != nil {
		// Cancel the context when write fails (connection closed)
		c.cancel()
		return 0, err
	}
	return len(p), nil
}

func (c *wsConn) Close() error {
	c.cancel() // Cancel the context when explicitly closed
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
