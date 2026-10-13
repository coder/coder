package agentbrowser

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/websocket"
)

// API exposes the browser stream on the agent HTTP API.
type API struct {
	logger  slog.Logger
	browser *Browser
}

// NewAPI creates the browser API.
func NewAPI(logger slog.Logger, browser *Browser) *API {
	return &API{logger: logger, browser: browser}
}

// Routes returns the router to mount at /api/v0/browser.
func (a *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/stream", a.handleStream)
	return r
}

// Close stops the browser.
func (a *API) Close() error {
	return a.browser.Close()
}

func (a *API) handleStream(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	v, err := a.browser.attach(ctx)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrBrowserNotFound) {
			status = http.StatusServiceUnavailable
		}
		a.logger.Warn(ctx, "failed to start browser", slog.Error(err))
		httpapi.Write(ctx, rw, status, codersdk.Response{
			Message: "Failed to start the workspace browser.",
			Detail:  err.Error(),
		})
		return
	}
	defer a.browser.detach(v)

	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		a.logger.Warn(ctx, "failed to accept browser websocket", slog.Error(err))
		return
	}
	// coderd relays this as a byte stream, so message sizes are set by its
	// copy buffer, not by our frames.
	conn.SetReadLimit(4 << 20)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	nc := websocket.NetConn(ctx, conn, websocket.MessageBinary)
	defer nc.Close()

	go func() {
		defer cancel()
		for {
			typ, payload, err := readFrame(nc)
			if err != nil {
				return
			}
			if typ != frameTypeJSON {
				continue
			}
			var msg ClientMessage
			if err := json.Unmarshal(payload, &msg); err != nil {
				continue
			}
			a.browser.handleMessage(v, msg)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-v.closed:
			// Flush whatever was queued, such as a final error.
			for {
				select {
				case frame := <-v.send:
					if _, err := nc.Write(frame); err != nil {
						return
					}
				default:
					return
				}
			}
		case frame := <-v.send:
			if _, err := nc.Write(frame); err != nil {
				return
			}
		}
	}
}
