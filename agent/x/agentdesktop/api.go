package agentdesktop

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
	"github.com/coder/websocket"
)

// DesktopAction is the request body for the desktop action endpoint.
type DesktopAction struct {
	Action          string  `json:"action"`
	Coordinate      *[2]int `json:"coordinate,omitempty"`
	StartCoordinate *[2]int `json:"start_coordinate,omitempty"`
	Text            *string `json:"text,omitempty"`
	Duration        *int    `json:"duration,omitempty"`
	ScrollAmount    *int    `json:"scroll_amount,omitempty"`
	ScrollDirection *string `json:"scroll_direction,omitempty"`
	// ScaledWidth and ScaledHeight describe the declared model-facing desktop
	// geometry. When provided, input coordinates are mapped from declared space
	// to native desktop pixels before dispatching.
	ScaledWidth  *int `json:"scaled_width,omitempty"`
	ScaledHeight *int `json:"scaled_height,omitempty"`
}

// DesktopActionResponse is the response from the desktop action
// endpoint.
type DesktopActionResponse struct {
	Output           string `json:"output,omitempty"`
	ScreenshotData   string `json:"screenshot_data,omitempty"`
	ScreenshotWidth  int    `json:"screenshot_width,omitempty"`
	ScreenshotHeight int    `json:"screenshot_height,omitempty"`
}

// API exposes the desktop streaming HTTP routes for the agent.
type API struct {
	logger  slog.Logger
	desktop Desktop
	clock   quartz.Clock

	closeMu sync.Mutex
	closed  bool
}

// NewAPI creates a new desktop streaming API.
func NewAPI(logger slog.Logger, desktop Desktop, clock quartz.Clock) *API {
	if clock == nil {
		clock = quartz.NewReal()
	}
	return &API{
		logger:  logger,
		desktop: desktop,
		clock:   clock,
	}
}

// Routes returns the chi router for mounting at /api/v0/desktop.
func (a *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/vnc", a.handleDesktopVNC)
	r.Post("/action", a.handleAction)
	r.Route("/recording", func(r chi.Router) {
		r.Post("/start", a.handleRecordingStart)
		r.Post("/stop", a.handleRecordingStop)
	})
	return r
}

func (a *API) handleDesktopVNC(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Start the desktop session (idempotent).
	_, err := a.desktop.Start(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to start desktop session.",
			Detail:  err.Error(),
		})
		return
	}

	// Get a VNC connection.
	vncConn, err := a.desktop.VNCConn(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to connect to VNC server.",
			Detail:  err.Error(),
		})
		return
	}
	defer vncConn.Close()

	// Accept WebSocket from coderd.
	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		a.logger.Error(ctx, "failed to accept websocket", slog.Error(err))
		return
	}

	// No read limit — RFB framebuffer updates can be large.
	conn.SetReadLimit(-1)

	wsCtx, wsNetConn := codersdk.WebsocketNetConn(ctx, conn, websocket.MessageBinary)
	defer wsNetConn.Close()

	// Bicopy raw bytes between WebSocket and VNC TCP.
	agentssh.Bicopy(wsCtx, wsNetConn, vncConn)
}

func (a *API) handleAction(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	handlerStart := a.clock.Now()

	// Update last desktop action timestamp for idle recording monitor.
	a.desktop.RecordActivity()

	// Ensure the desktop is running and grab native dimensions.
	cfg, err := a.desktop.Start(ctx)
	if err != nil {
		a.logger.Warn(ctx, "handleAction: desktop.Start failed",
			slog.Error(err),
			slog.F("elapsed_ms", a.clock.Since(handlerStart).Milliseconds()),
		)
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to start desktop session.",
			Detail:  err.Error(),
		})
		return
	}

	var action DesktopAction
	if err := json.NewDecoder(r.Body).Decode(&action); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to decode request body.",
			Detail:  err.Error(),
		})
		return
	}

	a.logger.Info(ctx, "handleAction: started",
		slog.F("action", action.Action),
		slog.F("elapsed_ms", a.clock.Since(handlerStart).Milliseconds()),
	)

	geometry := desktopGeometryForAction(cfg, action)
	scaleXY := geometry.DeclaredPointToNative

	var resp DesktopActionResponse

	switch action.Action {
	case "key":
		if action.Text == nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Missing \"text\" for key action.",
			})
			return
		}
		if err := a.desktop.KeyPress(ctx, *action.Text); err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Key press failed.",
				Detail:  err.Error(),
			})
			return
		}
		resp.Output = "key action performed"

	case "type":
		if action.Text == nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Missing \"text\" for type action.",
			})
			return
		}
		if err := a.desktop.Type(ctx, *action.Text); err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Type action failed.",
				Detail:  err.Error(),
			})
			return
		}
		resp.Output = "type action performed"

	case "cursor_position":
		nativeX, nativeY, err := a.desktop.CursorPosition(ctx)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Cursor position failed.",
				Detail:  err.Error(),
			})
			return
		}
		x, y := geometry.NativePointToDeclared(nativeX, nativeY)
		resp.Output = "x=" + strconv.Itoa(x) + ",y=" + strconv.Itoa(y)

	case "mouse_move":
		x, y, err := coordFromAction(action)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: err.Error(),
			})
			return
		}
		x, y = scaleXY(x, y)
		if err := a.desktop.Move(ctx, x, y); err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Mouse move failed.",
				Detail:  err.Error(),
			})
			return
		}
		resp.Output = "mouse_move action performed"

	case "left_click":
		x, y, err := coordFromAction(action)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: err.Error(),
			})
			return
		}
		x, y = scaleXY(x, y)
		stepStart := a.clock.Now()
		if err := a.desktop.Click(ctx, x, y, MouseButtonLeft); err != nil {
			a.logger.Warn(ctx, "handleAction: Click failed",
				slog.F("action", "left_click"),
				slog.F("step", "click"),
				slog.F("step_ms", time.Since(stepStart).Milliseconds()),
				slog.F("elapsed_ms", a.clock.Since(handlerStart).Milliseconds()),
				slog.Error(err),
			)
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Left click failed.",
				Detail:  err.Error(),
			})
			return
		}
		a.logger.Debug(ctx, "handleAction: Click completed",
			slog.F("action", "left_click"),
			slog.F("step_ms", time.Since(stepStart).Milliseconds()),
			slog.F("elapsed_ms", a.clock.Since(handlerStart).Milliseconds()),
		)
		resp.Output = "left_click action performed"

	case "left_click_drag":
		if action.Coordinate == nil || action.StartCoordinate == nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Missing \"coordinate\" or \"start_coordinate\" for left_click_drag.",
			})
			return
		}
		sx, sy := scaleXY(action.StartCoordinate[0], action.StartCoordinate[1])
		ex, ey := scaleXY(action.Coordinate[0], action.Coordinate[1])
		if err := a.desktop.Drag(ctx, sx, sy, ex, ey); err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Left click drag failed.",
				Detail:  err.Error(),
			})
			return
		}
		resp.Output = "left_click_drag action performed"

	case "left_mouse_down":
		if err := a.desktop.ButtonDown(ctx, MouseButtonLeft); err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Left mouse down failed.",
				Detail:  err.Error(),
			})
			return
		}
		resp.Output = "left_mouse_down action performed"

	case "left_mouse_up":
		if err := a.desktop.ButtonUp(ctx, MouseButtonLeft); err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Left mouse up failed.",
				Detail:  err.Error(),
			})
			return
		}
		resp.Output = "left_mouse_up action performed"

	case "right_click":
		x, y, err := coordFromAction(action)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: err.Error(),
			})
			return
		}
		x, y = scaleXY(x, y)
		if err := a.desktop.Click(ctx, x, y, MouseButtonRight); err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Right click failed.",
				Detail:  err.Error(),
			})
			return
		}
		resp.Output = "right_click action performed"

	case "middle_click":
		x, y, err := coordFromAction(action)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: err.Error(),
			})
			return
		}
		x, y = scaleXY(x, y)
		if err := a.desktop.Click(ctx, x, y, MouseButtonMiddle); err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Middle click failed.",
				Detail:  err.Error(),
			})
			return
		}
		resp.Output = "middle_click action performed"

	case "double_click":
		x, y, err := coordFromAction(action)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: err.Error(),
			})
			return
		}
		x, y = scaleXY(x, y)
		if err := a.desktop.DoubleClick(ctx, x, y, MouseButtonLeft); err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Double click failed.",
				Detail:  err.Error(),
			})
			return
		}
		resp.Output = "double_click action performed"

	case "triple_click":
		x, y, err := coordFromAction(action)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: err.Error(),
			})
			return
		}
		x, y = scaleXY(x, y)
		for range 3 {
			if err := a.desktop.Click(ctx, x, y, MouseButtonLeft); err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Triple click failed.",
					Detail:  err.Error(),
				})
				return
			}
		}
		resp.Output = "triple_click action performed"

	case "scroll":
		x, y, err := coordFromAction(action)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: err.Error(),
			})
			return
		}
		x, y = scaleXY(x, y)

		amount := 3
		if action.ScrollAmount != nil {
			amount = *action.ScrollAmount
		}
		direction := "down"
		if action.ScrollDirection != nil {
			direction = *action.ScrollDirection
		}

		var dx, dy int
		switch direction {
		case "up":
			dy = -amount
		case "down":
			dy = amount
		case "left":
			dx = -amount
		case "right":
			dx = amount
		default:
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid scroll direction: " + direction,
			})
			return
		}

		if err := a.desktop.Scroll(ctx, x, y, dx, dy); err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Scroll failed.",
				Detail:  err.Error(),
			})
			return
		}
		resp.Output = "scroll action performed"

	case "hold_key":
		if action.Text == nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Missing \"text\" for hold_key action.",
			})
			return
		}
		dur := 1000
		if action.Duration != nil {
			dur = *action.Duration
		}
		if err := a.desktop.KeyDown(ctx, *action.Text); err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Key down failed.",
				Detail:  err.Error(),
			})
			return
		}
		timer := a.clock.NewTimer(time.Duration(dur)*time.Millisecond, "agentdesktop", "hold_key")
		defer timer.Stop()
		select {
		case <-ctx.Done():
			// Context canceled; release the key immediately.
			if err := a.desktop.KeyUp(ctx, *action.Text); err != nil {
				a.logger.Warn(ctx, "handleAction: KeyUp after context cancel", slog.Error(err))
			}
			return
		case <-timer.C:
		}
		if err := a.desktop.KeyUp(ctx, *action.Text); err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Key up failed.",
				Detail:  err.Error(),
			})
			return
		}
		resp.Output = "hold_key action performed"

	case "screenshot":
		result, err := a.desktop.Screenshot(ctx, ScreenshotOptions{
			TargetWidth:  geometry.DeclaredWidth,
			TargetHeight: geometry.DeclaredHeight,
		})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Screenshot failed.",
				Detail:  err.Error(),
			})
			return
		}
		resp.Output = "screenshot"
		resp.ScreenshotData = result.Data
		resp.ScreenshotWidth = geometry.DeclaredWidth
		resp.ScreenshotHeight = geometry.DeclaredHeight

	default:
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Unknown action: " + action.Action,
		})
		return
	}

	elapsedMs := a.clock.Since(handlerStart).Milliseconds()
	if ctx.Err() != nil {
		a.logger.Error(ctx, "handleAction: context canceled before writing response",
			slog.F("action", action.Action),
			slog.F("elapsed_ms", elapsedMs),
			slog.Error(ctx.Err()),
		)
		return
	}
	a.logger.Info(ctx, "handleAction: writing response",
		slog.F("action", action.Action),
		slog.F("elapsed_ms", elapsedMs),
	)
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// Close shuts down the desktop session if one is running.
func (a *API) Close() error {
	a.closeMu.Lock()
	if a.closed {
		a.closeMu.Unlock()
		return nil
	}
	a.closed = true
	a.closeMu.Unlock()

	return a.desktop.Close()
}

// decodeRecordingRequest decodes and validates a recording request
// from the HTTP body, returning the recording ID. Returns false if
// the request was invalid and an error response was already written.
func (*API) decodeRecordingRequest(rw http.ResponseWriter, r *http.Request) (string, bool) {
	ctx := r.Context()
	var req struct {
		RecordingID string `json:"recording_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to decode request body.",
			Detail:  err.Error(),
		})
		return "", false
	}
	if req.RecordingID == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Missing recording_id.",
		})
		return "", false
	}
	if _, err := uuid.Parse(req.RecordingID); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid recording_id format.",
			Detail:  "recording_id must be a valid UUID.",
		})
		return "", false
	}
	return req.RecordingID, true
}

func (a *API) handleRecordingStart(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	recordingID, ok := a.decodeRecordingRequest(rw, r)
	if !ok {
		return
	}

	a.closeMu.Lock()
	if a.closed {
		a.closeMu.Unlock()
		httpapi.Write(ctx, rw, http.StatusServiceUnavailable, codersdk.Response{
			Message: "Desktop API is shutting down.",
		})
		return
	}
	a.closeMu.Unlock()

	if err := a.desktop.StartRecording(ctx, recordingID); err != nil {
		if errors.Is(err, ErrDesktopClosed) {
			httpapi.Write(ctx, rw, http.StatusServiceUnavailable, codersdk.Response{
				Message: "Desktop API is shutting down.",
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to start recording.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: "Recording started.",
	})
}

func (a *API) handleRecordingStop(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	recordingID, ok := a.decodeRecordingRequest(rw, r)
	if !ok {
		return
	}

	a.closeMu.Lock()
	if a.closed {
		a.closeMu.Unlock()
		httpapi.Write(ctx, rw, http.StatusServiceUnavailable, codersdk.Response{
			Message: "Desktop API is shutting down.",
		})
		return
	}
	a.closeMu.Unlock()

	// Stop recording (idempotent).
	// Use a context detached from the HTTP request so that if the
	// connection drops, the recording process can still shut down
	// gracefully. WithoutCancel preserves request-scoped values.
	stopCtx, stopCancel := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Second)
	defer stopCancel()
	artifact, err := a.desktop.StopRecording(stopCtx, recordingID)
	if err != nil {
		if errors.Is(err, ErrUnknownRecording) {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: "Recording not found.",
				Detail:  err.Error(),
			})
			return
		}
		if errors.Is(err, ErrRecordingCorrupted) {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Recording is corrupted.",
				Detail:  err.Error(),
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to stop recording.",
			Detail:  err.Error(),
		})
		return
	}
	defer artifact.Reader.Close()

	if artifact.Size > workspacesdk.MaxRecordingSize {
		a.logger.Warn(ctx, "recording file exceeds maximum size",
			slog.F("recording_id", recordingID),
			slog.F("size", artifact.Size),
			slog.F("max_size", workspacesdk.MaxRecordingSize),
		)
		httpapi.Write(ctx, rw, http.StatusRequestEntityTooLarge, codersdk.Response{
			Message: "Recording file exceeds maximum allowed size.",
		})
		return
	}

	rw.Header().Set("Content-Type", "video/mp4")
	rw.Header().Set("Content-Length", strconv.FormatInt(artifact.Size, 10))
	rw.WriteHeader(http.StatusOK)
	_, _ = io.Copy(rw, artifact.Reader)
}

// coordFromAction extracts the coordinate pair from a DesktopAction,
// returning an error if the coordinate field is missing.
func coordFromAction(action DesktopAction) (x, y int, err error) {
	if action.Coordinate == nil {
		return 0, 0, &missingFieldError{field: "coordinate", action: action.Action}
	}
	return action.Coordinate[0], action.Coordinate[1], nil
}

func desktopGeometryForAction(cfg DisplayConfig, action DesktopAction) workspacesdk.DesktopGeometry {
	declaredWidth := cfg.Width
	declaredHeight := cfg.Height
	if action.ScaledWidth != nil && *action.ScaledWidth > 0 {
		declaredWidth = *action.ScaledWidth
	}
	if action.ScaledHeight != nil && *action.ScaledHeight > 0 {
		declaredHeight = *action.ScaledHeight
	}
	return workspacesdk.NewDesktopGeometryWithDeclared(
		cfg.Width,
		cfg.Height,
		declaredWidth,
		declaredHeight,
	)
}

// missingFieldError is returned when a required field is absent from
// a DesktopAction.
type missingFieldError struct {
	field  string
	action string
}

func (e *missingFieldError) Error() string {
	return "Missing \"" + e.field + "\" for " + e.action + " action."
}
