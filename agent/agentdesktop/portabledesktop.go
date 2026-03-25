package agentdesktop

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// portableDesktopOutput is the JSON output from
// `portabledesktop up --json`.
type portableDesktopOutput struct {
	VNCPort  int    `json:"vncPort"`
	Geometry string `json:"geometry"` // e.g. "1920x1080"
}

// desktopSession tracks a running portabledesktop process.
type desktopSession struct {
	cmd     *exec.Cmd
	vncPort int
	width   int // native width, parsed from geometry
	height  int // native height, parsed from geometry
	display int // X11 display number, -1 if not available
	cancel  context.CancelFunc
}

// cursorOutput is the JSON output from `portabledesktop cursor --json`.
type cursorOutput struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// screenshotOutput is the JSON output from
// `portabledesktop screenshot --json`.
type screenshotOutput struct {
	Data string `json:"data"`
}

// portableDesktop implements Desktop by shelling out to the
// portabledesktop CLI via agentexec.Execer.
type portableDesktop struct {
	logger       slog.Logger
	execer       agentexec.Execer
	scriptBinDir string // coder script bin directory

	mu      sync.Mutex
	session *desktopSession // nil until started
	binPath string          // resolved path to binary, cached
	closed  bool
}

// NewPortableDesktop creates a Desktop backed by the portabledesktop
// CLI binary, using execer to spawn child processes. scriptBinDir is
// the coder script bin directory checked for the binary.
func NewPortableDesktop(
	logger slog.Logger,
	execer agentexec.Execer,
	scriptBinDir string,
) Desktop {
	return &portableDesktop{
		logger:       logger,
		execer:       execer,
		scriptBinDir: scriptBinDir,
	}
}

// Start launches the desktop session (idempotent).
func (p *portableDesktop) Start(ctx context.Context) (DisplayConfig, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return DisplayConfig{}, xerrors.New("desktop is closed")
	}

	if err := p.ensureBinary(ctx); err != nil {
		return DisplayConfig{}, xerrors.Errorf("ensure portabledesktop binary: %w", err)
	}

	// If we have an existing session, check if it's still alive.
	if p.session != nil {
		if !(p.session.cmd.ProcessState != nil && p.session.cmd.ProcessState.Exited()) {
			return DisplayConfig{
				Width:   p.session.width,
				Height:  p.session.height,
				VNCPort: p.session.vncPort,
				Display: p.session.display,
			}, nil
		}
		// Process died — clean up and recreate.
		p.logger.Warn(ctx, "portabledesktop process died, recreating session")
		p.session.cancel()
		p.session = nil
	}

	// Spawn portabledesktop up --json.
	sessionCtx, sessionCancel := context.WithCancel(context.Background())

	//nolint:gosec // portabledesktop is a trusted binary resolved via ensureBinary.
	cmd := p.execer.CommandContext(sessionCtx, p.binPath, "up", "--json",
		"--geometry", fmt.Sprintf("%dx%d", workspacesdk.DesktopNativeWidth, workspacesdk.DesktopNativeHeight))
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		sessionCancel()
		return DisplayConfig{}, xerrors.Errorf("create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		sessionCancel()
		return DisplayConfig{}, xerrors.Errorf("start portabledesktop: %w", err)
	}

	// Parse the JSON output to get VNC port and geometry.
	var output portableDesktopOutput
	if err := json.NewDecoder(stdout).Decode(&output); err != nil {
		sessionCancel()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return DisplayConfig{}, xerrors.Errorf("parse portabledesktop output: %w", err)
	}

	if output.VNCPort == 0 {
		sessionCancel()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return DisplayConfig{}, xerrors.New("portabledesktop returned port 0")
	}

	var w, h int
	if output.Geometry != "" {
		if _, err := fmt.Sscanf(output.Geometry, "%dx%d", &w, &h); err != nil {
			p.logger.Warn(ctx, "failed to parse geometry, using defaults",
				slog.F("geometry", output.Geometry),
				slog.Error(err),
			)
		}
	}

	p.logger.Info(ctx, "started portabledesktop session",
		slog.F("vnc_port", output.VNCPort),
		slog.F("width", w),
		slog.F("height", h),
		slog.F("pid", cmd.Process.Pid),
	)

	p.session = &desktopSession{
		cmd:     cmd,
		vncPort: output.VNCPort,
		width:   w,
		height:  h,
		display: -1,
		cancel:  sessionCancel,
	}

	return DisplayConfig{
		Width:   w,
		Height:  h,
		VNCPort: output.VNCPort,
		Display: -1,
	}, nil
}

// VNCConn dials the desktop's VNC server and returns a raw
// net.Conn carrying RFB binary frames.
func (p *portableDesktop) VNCConn(_ context.Context) (net.Conn, error) {
	p.mu.Lock()
	session := p.session
	p.mu.Unlock()

	if session == nil {
		return nil, xerrors.New("desktop session not started")
	}

	return net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", session.vncPort))
}

// Screenshot captures the current framebuffer as a base64-encoded PNG.
func (p *portableDesktop) Screenshot(ctx context.Context, opts ScreenshotOptions) (ScreenshotResult, error) {
	args := []string{"screenshot", "--json"}
	if opts.TargetWidth > 0 {
		args = append(args, "--target-width", strconv.Itoa(opts.TargetWidth))
	}
	if opts.TargetHeight > 0 {
		args = append(args, "--target-height", strconv.Itoa(opts.TargetHeight))
	}

	out, err := p.runCmd(ctx, args...)
	if err != nil {
		return ScreenshotResult{}, err
	}

	var result screenshotOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return ScreenshotResult{}, xerrors.Errorf("parse screenshot output: %w", err)
	}

	return ScreenshotResult(result), nil
}

// Move moves the mouse cursor to absolute coordinates.
func (p *portableDesktop) Move(ctx context.Context, x, y int) error {
	_, err := p.runCmd(ctx, "mouse", "move", strconv.Itoa(x), strconv.Itoa(y))
	return err
}

// Click performs a mouse button click at the given coordinates.
func (p *portableDesktop) Click(ctx context.Context, x, y int, button MouseButton) error {
	if _, err := p.runCmd(ctx, "mouse", "move", strconv.Itoa(x), strconv.Itoa(y)); err != nil {
		return err
	}
	_, err := p.runCmd(ctx, "mouse", "click", string(button))
	return err
}

// DoubleClick performs a double-click at the given coordinates.
func (p *portableDesktop) DoubleClick(ctx context.Context, x, y int, button MouseButton) error {
	if _, err := p.runCmd(ctx, "mouse", "move", strconv.Itoa(x), strconv.Itoa(y)); err != nil {
		return err
	}
	if _, err := p.runCmd(ctx, "mouse", "click", string(button)); err != nil {
		return err
	}
	_, err := p.runCmd(ctx, "mouse", "click", string(button))
	return err
}

// ButtonDown presses and holds a mouse button.
func (p *portableDesktop) ButtonDown(ctx context.Context, button MouseButton) error {
	_, err := p.runCmd(ctx, "mouse", "down", string(button))
	return err
}

// ButtonUp releases a mouse button.
func (p *portableDesktop) ButtonUp(ctx context.Context, button MouseButton) error {
	_, err := p.runCmd(ctx, "mouse", "up", string(button))
	return err
}

// Scroll scrolls by (dx, dy) clicks at the given coordinates.
func (p *portableDesktop) Scroll(ctx context.Context, x, y, dx, dy int) error {
	if _, err := p.runCmd(ctx, "mouse", "move", strconv.Itoa(x), strconv.Itoa(y)); err != nil {
		return err
	}
	_, err := p.runCmd(ctx, "mouse", "scroll", strconv.Itoa(dx), strconv.Itoa(dy))
	return err
}

// Drag moves from (startX,startY) to (endX,endY) while holding the
// left mouse button.
func (p *portableDesktop) Drag(ctx context.Context, startX, startY, endX, endY int) error {
	if _, err := p.runCmd(ctx, "mouse", "move", strconv.Itoa(startX), strconv.Itoa(startY)); err != nil {
		return err
	}
	if _, err := p.runCmd(ctx, "mouse", "down", string(MouseButtonLeft)); err != nil {
		return err
	}
	if _, err := p.runCmd(ctx, "mouse", "move", strconv.Itoa(endX), strconv.Itoa(endY)); err != nil {
		return err
	}
	_, err := p.runCmd(ctx, "mouse", "up", string(MouseButtonLeft))
	return err
}

// KeyPress sends a key-down then key-up for a key combo string.
func (p *portableDesktop) KeyPress(ctx context.Context, keys string) error {
	_, err := p.runCmd(ctx, "keyboard", "key", keys)
	return err
}

// KeyDown presses and holds a key.
func (p *portableDesktop) KeyDown(ctx context.Context, key string) error {
	_, err := p.runCmd(ctx, "keyboard", "down", key)
	return err
}

// KeyUp releases a key.
func (p *portableDesktop) KeyUp(ctx context.Context, key string) error {
	_, err := p.runCmd(ctx, "keyboard", "up", key)
	return err
}

// Type types a string of text character-by-character.
func (p *portableDesktop) Type(ctx context.Context, text string) error {
	_, err := p.runCmd(ctx, "keyboard", "type", text)
	return err
}

// CursorPosition returns the current cursor coordinates.
func (p *portableDesktop) CursorPosition(ctx context.Context) (x int, y int, err error) {
	out, err := p.runCmd(ctx, "cursor", "--json")
	if err != nil {
		return 0, 0, err
	}

	var result cursorOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return 0, 0, xerrors.Errorf("parse cursor output: %w", err)
	}

	return result.X, result.Y, nil
}

// Close shuts down the desktop session and cleans up resources.
func (p *portableDesktop) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closed = true
	if p.session != nil {
		p.session.cancel()
		// Xvnc is a child process — killing it cleans up the X
		// session.
		_ = p.session.cmd.Process.Kill()
		_ = p.session.cmd.Wait()
		p.session = nil
	}
	return nil
}

// runCmd executes a portabledesktop subcommand and returns combined
// output. The caller must have previously called ensureBinary.
func (p *portableDesktop) runCmd(ctx context.Context, args ...string) (string, error) {
	start := time.Now()
	//nolint:gosec // args are constructed by the caller, not user input.
	cmd := p.execer.CommandContext(ctx, p.binPath, args...)
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	if err != nil {
		p.logger.Warn(ctx, "portabledesktop command failed",
			slog.F("args", args),
			slog.F("elapsed_ms", elapsed.Milliseconds()),
			slog.Error(err),
			slog.F("output", string(out)),
		)
		return "", xerrors.Errorf("portabledesktop %s: %w: %s", args[0], err, string(out))
	}
	if elapsed > 5*time.Second {
		p.logger.Warn(ctx, "portabledesktop command slow",
			slog.F("args", args),
			slog.F("elapsed_ms", elapsed.Milliseconds()),
		)
	} else {
		p.logger.Debug(ctx, "portabledesktop command completed",
			slog.F("args", args),
			slog.F("elapsed_ms", elapsed.Milliseconds()),
		)
	}
	return string(out), nil
}

// ensureBinary resolves the portabledesktop binary from PATH or the
// coder script bin directory. It must be called while p.mu is held.
func (p *portableDesktop) ensureBinary(ctx context.Context) error {
	if p.binPath != "" {
		return nil
	}

	// 1. Check PATH.
	if path, err := exec.LookPath("portabledesktop"); err == nil {
		p.logger.Info(ctx, "found portabledesktop in PATH",
			slog.F("path", path),
		)
		p.binPath = path
		return nil
	}

	// 2. Check the coder script bin directory.
	scriptBinPath := filepath.Join(p.scriptBinDir, "portabledesktop")
	if info, err := os.Stat(scriptBinPath); err == nil && !info.IsDir() {
		// On Windows, permission bits don't indicate executability,
		// so accept any regular file.
		if runtime.GOOS == "windows" || info.Mode()&0o111 != 0 {
			p.logger.Info(ctx, "found portabledesktop in script bin directory",
				slog.F("path", scriptBinPath),
			)
			p.binPath = scriptBinPath
			return nil
		}
		p.logger.Warn(ctx, "portabledesktop found in script bin directory but not executable",
			slog.F("path", scriptBinPath),
			slog.F("mode", info.Mode().String()),
		)
	}

	return xerrors.New("portabledesktop binary not found in PATH or script bin directory")
}
