package agentdesktop_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentdesktop"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

// Ensure fakeDesktop satisfies the Desktop interface at compile time.
var _ agentdesktop.Desktop = (*fakeDesktop)(nil)

// fakeDesktop is a minimal Desktop implementation for unit tests.
type fakeDesktop struct {
	startErr      error
	cursorPos     [2]int
	startCfg      agentdesktop.DisplayConfig
	vncConnErr    error
	screenshotErr error
	screenshotRes agentdesktop.ScreenshotResult
	lastShotOpts  agentdesktop.ScreenshotOptions
	closed        bool

	// Track calls for assertions.
	lastMove    [2]int
	lastClick   [3]int // x, y, button
	lastScroll  [4]int // x, y, dx, dy
	lastKey     string
	lastTyped   string
	lastKeyDown string
	lastKeyUp   string
}

func (f *fakeDesktop) Start(context.Context) (agentdesktop.DisplayConfig, error) {
	return f.startCfg, f.startErr
}

func (f *fakeDesktop) VNCConn(context.Context) (net.Conn, error) {
	return nil, f.vncConnErr
}

func (f *fakeDesktop) Screenshot(_ context.Context, opts agentdesktop.ScreenshotOptions) (agentdesktop.ScreenshotResult, error) {
	f.lastShotOpts = opts
	return f.screenshotRes, f.screenshotErr
}

func (f *fakeDesktop) Move(_ context.Context, x, y int) error {
	f.lastMove = [2]int{x, y}
	return nil
}

func (f *fakeDesktop) Click(_ context.Context, x, y int, _ agentdesktop.MouseButton) error {
	f.lastClick = [3]int{x, y, 1}
	return nil
}

func (f *fakeDesktop) DoubleClick(_ context.Context, x, y int, _ agentdesktop.MouseButton) error {
	f.lastClick = [3]int{x, y, 2}
	return nil
}

func (*fakeDesktop) ButtonDown(context.Context, agentdesktop.MouseButton) error { return nil }
func (*fakeDesktop) ButtonUp(context.Context, agentdesktop.MouseButton) error   { return nil }

func (f *fakeDesktop) Scroll(_ context.Context, x, y, dx, dy int) error {
	f.lastScroll = [4]int{x, y, dx, dy}
	return nil
}

func (*fakeDesktop) Drag(context.Context, int, int, int, int) error { return nil }

func (f *fakeDesktop) KeyPress(_ context.Context, key string) error {
	f.lastKey = key
	return nil
}

func (f *fakeDesktop) KeyDown(_ context.Context, key string) error {
	f.lastKeyDown = key
	return nil
}

func (f *fakeDesktop) KeyUp(_ context.Context, key string) error {
	f.lastKeyUp = key
	return nil
}

func (f *fakeDesktop) Type(_ context.Context, text string) error {
	f.lastTyped = text
	return nil
}

func (f *fakeDesktop) CursorPosition(context.Context) (x int, y int, err error) {
	return f.cursorPos[0], f.cursorPos[1], nil
}

func (f *fakeDesktop) Close() error {
	f.closed = true
	return nil
}

func TestHandleDesktopVNC_StartError(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{startErr: xerrors.New("no desktop")}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/vnc", nil)

	handler := api.Routes()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	var resp codersdk.Response
	err := json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "Failed to start desktop session.", resp.Message)
}

func TestHandleAction_Screenshot(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	geometry := workspacesdk.DefaultDesktopGeometry()
	fake := &fakeDesktop{
		startCfg: agentdesktop.DisplayConfig{
			Width:  geometry.NativeWidth,
			Height: geometry.NativeHeight,
		},
		screenshotRes: agentdesktop.ScreenshotResult{Data: "base64data"},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	body := agentdesktop.DesktopAction{Action: "screenshot"}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	handler := api.Routes()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var result agentdesktop.DesktopActionResponse
	err = json.NewDecoder(rr.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "screenshot", result.Output)
	assert.Equal(t, "base64data", result.ScreenshotData)
	assert.Equal(t, geometry.NativeWidth, result.ScreenshotWidth)
	assert.Equal(t, geometry.NativeHeight, result.ScreenshotHeight)
	assert.Equal(t, agentdesktop.ScreenshotOptions{
		TargetWidth:  geometry.NativeWidth,
		TargetHeight: geometry.NativeHeight,
	}, fake.lastShotOpts)
}

func TestHandleAction_ScreenshotUsesDeclaredDimensionsFromRequest(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg:      agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
		screenshotRes: agentdesktop.ScreenshotResult{Data: "base64data"},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	sw := 1280
	sh := 720
	body := agentdesktop.DesktopAction{
		Action:       "screenshot",
		ScaledWidth:  &sw,
		ScaledHeight: &sh,
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	handler := api.Routes()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, agentdesktop.ScreenshotOptions{TargetWidth: 1280, TargetHeight: 720}, fake.lastShotOpts)

	var result agentdesktop.DesktopActionResponse
	err = json.NewDecoder(rr.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, 1280, result.ScreenshotWidth)
	assert.Equal(t, 720, result.ScreenshotHeight)
}

func TestHandleAction_LeftClick(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	body := agentdesktop.DesktopAction{
		Action:     "left_click",
		Coordinate: &[2]int{100, 200},
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	handler := api.Routes()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp agentdesktop.DesktopActionResponse
	err = json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "left_click action performed", resp.Output)
	assert.Equal(t, [3]int{100, 200, 1}, fake.lastClick)
}

func TestHandleAction_UnknownAction(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	body := agentdesktop.DesktopAction{Action: "explode"}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	handler := api.Routes()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleAction_KeyAction(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	text := "Return"
	body := agentdesktop.DesktopAction{
		Action: "key",
		Text:   &text,
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	handler := api.Routes()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "Return", fake.lastKey)
}

func TestHandleAction_TypeAction(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	text := "hello world"
	body := agentdesktop.DesktopAction{
		Action: "type",
		Text:   &text,
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	handler := api.Routes()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "hello world", fake.lastTyped)
}

func TestHandleAction_HoldKey(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
	}
	mClk := quartz.NewMock(t)
	trap := mClk.Trap().NewTimer("agentdesktop", "hold_key")
	defer trap.Close()
	api := agentdesktop.NewAPI(logger, fake, mClk)
	defer api.Close()

	text := "Shift_L"
	dur := 100
	body := agentdesktop.DesktopAction{
		Action:   "hold_key",
		Text:     &text,
		Duration: &dur,
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	handler := api.Routes()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(rr, req)
	}()

	trap.MustWait(req.Context()).MustRelease(req.Context())
	mClk.Advance(time.Duration(dur) * time.Millisecond).MustWait(req.Context())

	<-done

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp agentdesktop.DesktopActionResponse
	err = json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "hold_key action performed", resp.Output)
	assert.Equal(t, "Shift_L", fake.lastKeyDown)
	assert.Equal(t, "Shift_L", fake.lastKeyUp)
}

func TestHandleAction_HoldKeyMissingText(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	body := agentdesktop.DesktopAction{Action: "hold_key"}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	handler := api.Routes()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)

	var resp codersdk.Response
	err = json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "Missing \"text\" for hold_key action.", resp.Message)
}

func TestHandleAction_ScrollDown(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	dir := "down"
	amount := 5
	body := agentdesktop.DesktopAction{
		Action:          "scroll",
		Coordinate:      &[2]int{500, 400},
		ScrollDirection: &dir,
		ScrollAmount:    &amount,
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	handler := api.Routes()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, [4]int{500, 400, 0, 5}, fake.lastScroll)
}

func TestHandleAction_CoordinateScaling(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	sw := 1280
	sh := 720
	body := agentdesktop.DesktopAction{
		Action:       "mouse_move",
		Coordinate:   &[2]int{640, 360},
		ScaledWidth:  &sw,
		ScaledHeight: &sh,
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	handler := api.Routes()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, 960, fake.lastMove[0])
	assert.Equal(t, 540, fake.lastMove[1])
}

func TestHandleAction_CoordinateScalingClampsToLastPixel(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	sw := 1366
	sh := 768
	body := agentdesktop.DesktopAction{
		Action:       "mouse_move",
		Coordinate:   &[2]int{1365, 767},
		ScaledWidth:  &sw,
		ScaledHeight: &sh,
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	handler := api.Routes()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, 1919, fake.lastMove[0])
	assert.Equal(t, 1079, fake.lastMove[1])
}

func TestClose_DelegatesToDesktop(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{}
	api := agentdesktop.NewAPI(logger, fake, nil)

	err := api.Close()
	require.NoError(t, err)
	assert.True(t, fake.closed)
}

func TestClose_PreventsNewSessions(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{}
	api := agentdesktop.NewAPI(logger, fake, nil)

	err := api.Close()
	require.NoError(t, err)

	fake.startErr = xerrors.New("desktop is closed")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/vnc", nil)

	handler := api.Routes()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestHandleAction_CursorPositionReturnsDeclaredCoordinates(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg:  agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
		cursorPos: [2]int{960, 540},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	sw := 1280
	sh := 720
	body := agentdesktop.DesktopAction{
		Action:       "cursor_position",
		ScaledWidth:  &sw,
		ScaledHeight: &sh,
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	handler := api.Routes()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp agentdesktop.DesktopActionResponse
	err = json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	// Native (960,540) in 1920x1080 should map to declared space in 1280x720.
	assert.Equal(t, "x=640,y=360", resp.Output)
}
