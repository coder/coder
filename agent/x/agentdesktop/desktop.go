package agentdesktop

import (
	"context"
	"io"
	"net"

	"golang.org/x/xerrors"
)

// Desktop abstracts a virtual desktop session running inside a workspace.
type Desktop interface {
	// Start launches the desktop session. It is idempotent — calling
	// Start on an already-running session returns the existing
	// config. The returned DisplayConfig describes the running
	// session.
	Start(ctx context.Context) (DisplayConfig, error)

	// VNCConn dials the desktop's VNC server and returns a raw
	// net.Conn carrying RFB binary frames. Each call returns a new
	// connection; multiple clients can connect simultaneously.
	// Start must be called before VNCConn.
	VNCConn(ctx context.Context) (net.Conn, error)

	// Screenshot captures the current framebuffer as a PNG and
	// returns it base64-encoded. TargetWidth/TargetHeight in opts
	// are the desired output dimensions (the implementation
	// rescales); pass 0 to use native resolution.
	Screenshot(ctx context.Context, opts ScreenshotOptions) (ScreenshotResult, error)

	// Mouse operations.

	// Move moves the mouse cursor to absolute coordinates.
	Move(ctx context.Context, x, y int) error
	// Click performs a mouse button click at the given coordinates.
	Click(ctx context.Context, x, y int, button MouseButton) error
	// DoubleClick performs a double-click at the given coordinates.
	DoubleClick(ctx context.Context, x, y int, button MouseButton) error
	// ButtonDown presses and holds a mouse button.
	ButtonDown(ctx context.Context, button MouseButton) error
	// ButtonUp releases a mouse button.
	ButtonUp(ctx context.Context, button MouseButton) error
	// Scroll scrolls by (dx, dy) clicks at the given coordinates.
	Scroll(ctx context.Context, x, y, dx, dy int) error
	// Drag moves from (startX,startY) to (endX,endY) while holding
	// the left mouse button.
	Drag(ctx context.Context, startX, startY, endX, endY int) error

	// Keyboard operations.

	// KeyPress sends a key-down then key-up for a key combo string
	// (e.g. "Return", "ctrl+c").
	KeyPress(ctx context.Context, keys string) error
	// KeyDown presses and holds a key.
	KeyDown(ctx context.Context, key string) error
	// KeyUp releases a key.
	KeyUp(ctx context.Context, key string) error
	// Type types a string of text character-by-character.
	Type(ctx context.Context, text string) error

	// CursorPosition returns the current cursor coordinates.
	CursorPosition(ctx context.Context) (x, y int, err error)

	// RecordActivity marks the desktop as having received user
	// interaction, resetting the idle-recording timer.
	RecordActivity()

	// StartRecording begins recording the desktop to an MP4 file
	// using the caller-provided recording ID. Safe to call
	// repeatedly - active recordings continue unchanged, stopped
	// recordings are discarded and restarted. Concurrent recordings
	// are supported.
	StartRecording(ctx context.Context, recordingID string) error

	// StopRecording finalizes the recording identified by the given
	// ID. Idempotent - safe to call on an already-stopped recording.
	// Returns a RecordingArtifact that the caller can stream. The
	// caller must close the artifact when done. Returns an error if
	// the recording ID is unknown.
	StopRecording(ctx context.Context, recordingID string) (*RecordingArtifact, error)

	// Close shuts down the desktop session and cleans up resources.
	Close() error
}

// ErrUnknownRecording is returned by StopRecording when the
// recording ID is not recognized.
var ErrUnknownRecording = xerrors.New("unknown recording ID")

// ErrDesktopClosed is returned when an operation is attempted on a
// closed desktop session.
var ErrDesktopClosed = xerrors.New("desktop closed")

// ErrRecordingCorrupted is returned by StopRecording when the
// recording process was force-killed and the artifact is likely
// incomplete or corrupt.
var ErrRecordingCorrupted = xerrors.New("recording corrupted: process was force-killed")

// RecordingArtifact is a finalized recording returned by StopRecording.
// The caller streams the artifact and must call Close when done. The
// artifact remains valid even if the same recording ID is restarted
// or the desktop is closed while the caller is reading.
type RecordingArtifact struct {
	// Reader is the MP4 content. Callers must close it when done.
	Reader io.ReadCloser
	// Size is the byte length of the MP4 content.
	Size int64
}

// DisplayConfig describes a running desktop session.
type DisplayConfig struct {
	Width   int // native width in pixels
	Height  int // native height in pixels
	VNCPort int // local TCP port for the VNC server
	Display int // X11 display number (e.g. 1 for :1), -1 if N/A
}

// MouseButton identifies a mouse button.
type MouseButton string

const (
	MouseButtonLeft   MouseButton = "left"
	MouseButtonRight  MouseButton = "right"
	MouseButtonMiddle MouseButton = "middle"
)

// ScreenshotOptions configures a screenshot capture.
type ScreenshotOptions struct {
	TargetWidth  int // 0 = native
	TargetHeight int // 0 = native
}

// ScreenshotResult is a captured screenshot.
type ScreenshotResult struct {
	Data string // base64-encoded PNG
}
