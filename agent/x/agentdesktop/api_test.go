package agentdesktop_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/x/agentdesktop"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

// Test recording UUIDs used across tests.
const (
	testRecIDDefault         = "870e1f02-8118-4300-a37e-4adb0117baf3"
	testRecIDStartIdempotent = "250a2ffb-a5e5-4c94-9754-4d6a4ab7ba20"
	testRecIDStopIdempotent  = "38f8a378-f98f-4758-a4ae-950b44cf989a"
	testRecIDConcurrentA     = "8dc173eb-23c6-4601-a485-b6dfb2a42c3a"
	testRecIDConcurrentB     = "fea490d4-70f0-4798-a181-29d65ce25ae1"
	testRecIDRestart         = "75173a0d-b018-4e2e-a771-defa3fc6af69"
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

	thumbnailData []byte // if set, StopRecording includes a thumbnail

	// Recording tracking (guarded by recMu).
	recMu         sync.Mutex
	recordings    map[string]string // ID → file path
	stopCalls     []string          // recording IDs passed to StopRecording
	recStopCh     chan string       // optional: signaled when StopRecording is called
	startCount    int               // incremented on each new recording start
	activityCount int               // incremented by RecordActivity
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

func (f *fakeDesktop) StartRecording(_ context.Context, recordingID string) error {
	f.recMu.Lock()
	defer f.recMu.Unlock()
	if f.recordings == nil {
		f.recordings = make(map[string]string)
	}
	if path, ok := f.recordings[recordingID]; ok {
		// Check if already stopped (file still exists but stop was
		// called). For the fake, a stopped recording means its ID
		// appears in stopCalls. In that case, remove the old file
		// and start fresh.
		stopped := slices.Contains(f.stopCalls, recordingID)
		if !stopped {
			// Active recording - no-op.
			return nil
		}
		// Completed recording - discard old file, start fresh.
		_ = os.Remove(path)
		delete(f.recordings, recordingID)
	}
	f.startCount++
	tmpFile, err := os.CreateTemp("", "fake-recording-*.mp4")
	if err != nil {
		return err
	}
	_, _ = tmpFile.Write([]byte(fmt.Sprintf("fake-mp4-data-%s-%d", recordingID, f.startCount)))
	_ = tmpFile.Close()
	f.recordings[recordingID] = tmpFile.Name()
	return nil
}

func (f *fakeDesktop) StopRecording(_ context.Context, recordingID string) (*agentdesktop.RecordingArtifact, error) {
	f.recMu.Lock()
	defer f.recMu.Unlock()
	if f.recordings == nil {
		return nil, agentdesktop.ErrUnknownRecording
	}
	path, ok := f.recordings[recordingID]
	if !ok {
		return nil, agentdesktop.ErrUnknownRecording
	}
	f.stopCalls = append(f.stopCalls, recordingID)
	if f.recStopCh != nil {
		select {
		case f.recStopCh <- recordingID:
		default:
		}
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	artifact := &agentdesktop.RecordingArtifact{
		Reader: file,
		Size:   info.Size(),
	}
	if f.thumbnailData != nil {
		artifact.ThumbnailReader = io.NopCloser(bytes.NewReader(f.thumbnailData))
		artifact.ThumbnailSize = int64(len(f.thumbnailData))
	}
	return artifact, nil
}

func (f *fakeDesktop) RecordActivity() {
	f.recMu.Lock()
	f.activityCount++
	f.recMu.Unlock()
}

func (f *fakeDesktop) Close() error {
	f.closed = true
	f.recMu.Lock()
	defer f.recMu.Unlock()
	for _, path := range f.recordings {
		_ = os.Remove(path)
	}
	return nil
}

// failStartRecordingDesktop wraps fakeDesktop and overrides
// StartRecording to always return an error.
type failStartRecordingDesktop struct {
	fakeDesktop
	startRecordingErr error
}

func (f *failStartRecordingDesktop) StartRecording(_ context.Context, _ string) error {
	return f.startRecordingErr
}

// corruptedStopDesktop wraps fakeDesktop and overrides
// StopRecording to always return ErrRecordingCorrupted.
type corruptedStopDesktop struct {
	fakeDesktop
}

func (*corruptedStopDesktop) StopRecording(_ context.Context, _ string) (*agentdesktop.RecordingArtifact, error) {
	return nil, agentdesktop.ErrRecordingCorrupted
}

// oversizedFakeDesktop wraps fakeDesktop and expands recording files
// beyond MaxRecordingSize when StopRecording is called.
type oversizedFakeDesktop struct {
	fakeDesktop
}

func (f *oversizedFakeDesktop) StopRecording(ctx context.Context, recordingID string) (*agentdesktop.RecordingArtifact, error) {
	artifact, err := f.fakeDesktop.StopRecording(ctx, recordingID)
	if err != nil {
		return nil, err
	}
	// Close the original reader since we're going to re-open after truncation.
	artifact.Reader.Close()

	// Look up the path from the fakeDesktop recordings.
	f.fakeDesktop.recMu.Lock()
	path := f.fakeDesktop.recordings[recordingID]
	f.fakeDesktop.recMu.Unlock()

	// Expand the file to exceed the maximum recording size.
	if err := os.Truncate(path, workspacesdk.MaxRecordingSize+1); err != nil {
		return nil, err
	}
	// Re-open the truncated file.
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &agentdesktop.RecordingArtifact{
		Reader: file,
		Size:   workspacesdk.MaxRecordingSize + 1,
	}, nil
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

func TestHandleAction_CallsRecordActivity(t *testing.T) {
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
	require.Equal(t, http.StatusOK, rr.Code)

	fake.recMu.Lock()
	count := fake.activityCount
	fake.recMu.Unlock()
	assert.Equal(t, 1, count, "handleAction should call RecordActivity exactly once")
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

func TestRecordingStartStop(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	handler := api.Routes()

	// Start recording.
	startBody, err := json.Marshal(map[string]string{"recording_id": testRecIDDefault})
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/recording/start", bytes.NewReader(startBody))
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	// Stop recording.
	stopBody, err := json.Marshal(map[string]string{"recording_id": testRecIDDefault})
	require.NoError(t, err)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/recording/stop", bytes.NewReader(stopBody))
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	parts := parseMultipartParts(t, rr.Header().Get("Content-Type"), rr.Body.Bytes())
	assert.Equal(t, []byte("fake-mp4-data-"+testRecIDDefault+"-1"), parts["video/mp4"])
}

func TestRecordingStartFails(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &failStartRecordingDesktop{
		fakeDesktop: fakeDesktop{
			startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
		},
		startRecordingErr: xerrors.New("start recording error"),
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	handler := api.Routes()

	body, err := json.Marshal(map[string]string{"recording_id": uuid.New().String()})
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/recording/start", bytes.NewReader(body))
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	var resp codersdk.Response
	err = json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "Failed to start recording.", resp.Message)
}

func TestRecordingStartIdempotent(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	handler := api.Routes()

	// Start same recording twice - both should succeed.
	for range 2 {
		body, err := json.Marshal(map[string]string{"recording_id": testRecIDStartIdempotent})
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/recording/start", bytes.NewReader(body))
		handler.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
	}

	// Stop once, verify normal response.
	stopBody, err := json.Marshal(map[string]string{"recording_id": testRecIDStartIdempotent})
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/recording/stop", bytes.NewReader(stopBody))
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	parts := parseMultipartParts(t, rr.Header().Get("Content-Type"), rr.Body.Bytes())
	assert.Equal(t, []byte("fake-mp4-data-"+testRecIDStartIdempotent+"-1"), parts["video/mp4"])
}

func TestRecordingStopIdempotent(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	handler := api.Routes()

	// Start recording.
	startBody, err := json.Marshal(map[string]string{"recording_id": testRecIDStopIdempotent})
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/recording/start", bytes.NewReader(startBody))
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	// Stop twice - both should succeed with identical data.
	var videoParts [2][]byte
	for i := range 2 {
		body, err := json.Marshal(map[string]string{"recording_id": testRecIDStopIdempotent})
		require.NoError(t, err)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/recording/stop", bytes.NewReader(body))
		handler.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusOK, recorder.Code)
		parts := parseMultipartParts(t, recorder.Header().Get("Content-Type"), recorder.Body.Bytes())
		videoParts[i] = parts["video/mp4"]
	}
	assert.Equal(t, videoParts[0], videoParts[1])
}

func TestRecordingStopInvalidIDFormat(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	handler := api.Routes()

	body, err := json.Marshal(map[string]string{"recording_id": "not-a-uuid"})
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/recording/stop", bytes.NewReader(body))
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestRecordingStopUnknownRecording(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	handler := api.Routes()

	// Send a valid UUID that was never started - should reach
	// StopRecording, get ErrUnknownRecording, and return 404.
	body, err := json.Marshal(map[string]string{"recording_id": uuid.New().String()})
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/recording/stop", bytes.NewReader(body))
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)

	var resp codersdk.Response
	err = json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "Recording not found.", resp.Message)
}

func TestRecordingStopOversizedFile(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &oversizedFakeDesktop{
		fakeDesktop: fakeDesktop{
			startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
		},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	handler := api.Routes()

	// Start recording.
	recID := uuid.New().String()
	startBody, err := json.Marshal(map[string]string{"recording_id": recID})
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/recording/start", bytes.NewReader(startBody))
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	// Stop recording - file exceeds max size, expect 413.
	stopBody, err := json.Marshal(map[string]string{"recording_id": recID})
	require.NoError(t, err)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/recording/stop", bytes.NewReader(stopBody))
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)

	var resp codersdk.Response
	err = json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "Recording file exceeds maximum allowed size.", resp.Message)
}

func TestRecordingMultipleSimultaneous(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	handler := api.Routes()

	// Start two recordings with different IDs.
	for _, id := range []string{testRecIDConcurrentA, testRecIDConcurrentB} {
		body, err := json.Marshal(map[string]string{"recording_id": id})
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/recording/start", bytes.NewReader(body))
		handler.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
	}

	// Stop both and verify each returns its own data.
	expected := map[string][]byte{
		testRecIDConcurrentA: []byte("fake-mp4-data-" + testRecIDConcurrentA + "-1"),
		testRecIDConcurrentB: []byte("fake-mp4-data-" + testRecIDConcurrentB + "-2"),
	}
	for _, id := range []string{testRecIDConcurrentA, testRecIDConcurrentB} {
		body, err := json.Marshal(map[string]string{"recording_id": id})
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/recording/stop", bytes.NewReader(body))
		handler.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
		parts := parseMultipartParts(t, rr.Header().Get("Content-Type"), rr.Body.Bytes())
		assert.Equal(t, expected[id], parts["video/mp4"])
	}
}

func TestRecordingStartMalformedBody(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	handler := api.Routes()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/recording/start", bytes.NewReader([]byte("not json")))
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestRecordingStartEmptyID(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	handler := api.Routes()

	body, err := json.Marshal(map[string]string{"recording_id": ""})
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/recording/start", bytes.NewReader(body))
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestRecordingStopEmptyID(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	handler := api.Routes()

	body, err := json.Marshal(map[string]string{"recording_id": ""})
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/recording/stop", bytes.NewReader(body))
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestRecordingStopMalformedBody(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	handler := api.Routes()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/recording/stop", bytes.NewReader([]byte("not json")))
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestRecordingStartAfterCompleted(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	handler := api.Routes()

	// Step 1: Start recording.
	startBody, err := json.Marshal(map[string]string{"recording_id": testRecIDRestart})
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/recording/start", bytes.NewReader(startBody))
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	// Step 2: Stop recording (gets first MP4 data).
	stopBody, err := json.Marshal(map[string]string{"recording_id": testRecIDRestart})
	require.NoError(t, err)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/recording/stop", bytes.NewReader(stopBody))
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	firstParts := parseMultipartParts(t, rr.Header().Get("Content-Type"), rr.Body.Bytes())
	firstData := firstParts["video/mp4"]
	require.NotEmpty(t, firstData)

	// Step 3: Start again with the same ID - should succeed
	// (old file discarded, new recording started).
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/recording/start", bytes.NewReader(startBody))
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	// Step 4: Stop again - should return NEW MP4 data.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/recording/stop", bytes.NewReader(stopBody))
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	secondParts := parseMultipartParts(t, rr.Header().Get("Content-Type"), rr.Body.Bytes())
	secondData := secondParts["video/mp4"]
	require.NotEmpty(t, secondData)

	// The two recordings should have different data because the
	// fake increments a counter on each fresh start.
	assert.NotEqual(t, firstData, secondData,
		"restarted recording should produce different data")
}

func TestRecordingStartAfterClose(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)

	handler := api.Routes()

	// Close the API before sending the request.
	api.Close()

	body, err := json.Marshal(map[string]string{"recording_id": uuid.New().String()})
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/recording/start", bytes.NewReader(body))
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)

	var resp codersdk.Response
	err = json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "Desktop API is shutting down.", resp.Message)
}

func TestRecordingStartDesktopClosed(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	// StartRecording returns ErrDesktopClosed to simulate a race
	// where the desktop is closed between the API-level check and
	// the desktop-level StartRecording call.
	fake := &failStartRecordingDesktop{
		fakeDesktop: fakeDesktop{
			startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
		},
		startRecordingErr: agentdesktop.ErrDesktopClosed,
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	handler := api.Routes()

	body, err := json.Marshal(map[string]string{"recording_id": uuid.New().String()})
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/recording/start", bytes.NewReader(body))
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)

	var resp codersdk.Response
	err = json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "Desktop API is shutting down.", resp.Message)
}

func TestRecordingStopCorrupted(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &corruptedStopDesktop{
		fakeDesktop: fakeDesktop{
			startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
		},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	handler := api.Routes()

	// Start a recording so the stop has something to find.
	recID := uuid.New().String()
	startBody, err := json.Marshal(map[string]string{"recording_id": recID})
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/recording/start", bytes.NewReader(startBody))
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	// Stop returns ErrRecordingCorrupted.
	stopBody, err := json.Marshal(map[string]string{"recording_id": recID})
	require.NoError(t, err)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/recording/stop", bytes.NewReader(stopBody))
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	var respStop codersdk.Response
	err = json.NewDecoder(rr.Body).Decode(&respStop)
	require.NoError(t, err)
	assert.Equal(t, "Recording is corrupted.", respStop.Message)
}

// parseMultipartParts parses a multipart/mixed response and returns
// a map from Content-Type to body bytes.
func parseMultipartParts(t *testing.T, contentType string, body []byte) map[string][]byte {
	t.Helper()
	_, params, err := mime.ParseMediaType(contentType)
	require.NoError(t, err, "parse Content-Type")
	boundary := params["boundary"]
	require.NotEmpty(t, boundary, "missing boundary")
	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	parts := make(map[string][]byte)
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err, "unexpected multipart parse error")
		ct := part.Header.Get("Content-Type")
		data, readErr := io.ReadAll(part)
		require.NoError(t, readErr)
		parts[ct] = data
	}
	return parts
}

func TestHandleRecordingStop_WithThumbnail(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	// Create a fake JPEG header: 0xFF 0xD8 0xFF followed by 509 zero bytes.
	thumbnail := make([]byte, 512)
	thumbnail[0] = 0xff
	thumbnail[1] = 0xd8
	thumbnail[2] = 0xff

	fake := &fakeDesktop{
		startCfg:      agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
		thumbnailData: thumbnail,
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	handler := api.Routes()

	// Start recording.
	recID := uuid.New().String()
	startBody, err := json.Marshal(map[string]string{"recording_id": recID})
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/recording/start", bytes.NewReader(startBody))
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	// Stop recording.
	stopBody, err := json.Marshal(map[string]string{"recording_id": recID})
	require.NoError(t, err)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/recording/stop", bytes.NewReader(stopBody))
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	// Verify multipart response.
	ct := rr.Header().Get("Content-Type")
	assert.True(t, strings.HasPrefix(ct, "multipart/mixed"),
		"expected multipart/mixed Content-Type, got %s", ct)

	parts := parseMultipartParts(t, ct, rr.Body.Bytes())
	assert.Len(t, parts, 2, "expected exactly 2 parts (video + thumbnail)")

	// The fake writes "fake-mp4-data-<id>-<counter>" as the MP4 content.
	expectedMP4 := []byte("fake-mp4-data-" + recID + "-1")
	assert.Equal(t, expectedMP4, parts["video/mp4"])
	assert.Equal(t, thumbnail, parts["image/jpeg"])
}

func TestHandleRecordingStop_NoThumbnail(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	fake := &fakeDesktop{
		startCfg: agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	handler := api.Routes()

	// Start recording.
	recID := uuid.New().String()
	startBody, err := json.Marshal(map[string]string{"recording_id": recID})
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/recording/start", bytes.NewReader(startBody))
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	// Stop recording.
	stopBody, err := json.Marshal(map[string]string{"recording_id": recID})
	require.NoError(t, err)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/recording/stop", bytes.NewReader(stopBody))
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	// Verify multipart response.
	ct := rr.Header().Get("Content-Type")
	assert.True(t, strings.HasPrefix(ct, "multipart/mixed"),
		"expected multipart/mixed Content-Type, got %s", ct)

	parts := parseMultipartParts(t, ct, rr.Body.Bytes())
	assert.Len(t, parts, 1, "expected exactly 1 part (video only)")

	expectedMP4 := []byte("fake-mp4-data-" + recID + "-1")
	assert.Equal(t, expectedMP4, parts["video/mp4"])
}

func TestHandleRecordingStop_OversizedThumbnail(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	// Create thumbnail data that exceeds MaxThumbnailSize.
	oversizedThumb := make([]byte, workspacesdk.MaxThumbnailSize+1)
	oversizedThumb[0] = 0xff
	oversizedThumb[1] = 0xd8
	oversizedThumb[2] = 0xff

	fake := &fakeDesktop{
		startCfg:      agentdesktop.DisplayConfig{Width: 1920, Height: 1080},
		thumbnailData: oversizedThumb,
	}
	api := agentdesktop.NewAPI(logger, fake, nil)
	defer api.Close()

	handler := api.Routes()

	// Start recording.
	recID := uuid.New().String()
	startBody, err := json.Marshal(map[string]string{"recording_id": recID})
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/recording/start", bytes.NewReader(startBody))
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	// Stop recording.
	stopBody, err := json.Marshal(map[string]string{"recording_id": recID})
	require.NoError(t, err)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/recording/stop", bytes.NewReader(stopBody))
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	// Verify multipart response contains only the video part.
	ct := rr.Header().Get("Content-Type")
	assert.True(t, strings.HasPrefix(ct, "multipart/mixed"),
		"expected multipart/mixed Content-Type, got %s", ct)

	parts := parseMultipartParts(t, ct, rr.Body.Bytes())
	assert.Len(t, parts, 1, "expected exactly 1 part (video only, oversized thumbnail discarded)")

	expectedMP4 := []byte("fake-mp4-data-" + recID + "-1")
	assert.Equal(t, expectedMP4, parts["video/mp4"])
}
