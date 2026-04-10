package agentdesktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
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

// recordingProcess tracks a single desktop recording subprocess.
type recordingProcess struct {
	cmd        *exec.Cmd
	filePath   string
	thumbPath  string
	stopped    bool
	killed     bool          // true when the process was SIGKILLed
	done       chan struct{} // closed when cmd.Wait() returns
	waitErr    error         // set before done is closed
	stopOnce   sync.Once
	idleCancel context.CancelFunc // cancels the per-recording idle goroutine
	idleDone   chan struct{}      // closed when idle goroutine exits
}

// maxConcurrentRecordings is the maximum number of active (non-stopped)
// recordings allowed at once. This prevents resource exhaustion.
const maxConcurrentRecordings = 5

// idleTimeout is the duration of desktop inactivity after which all
// active recordings are automatically stopped.
const idleTimeout = 10 * time.Minute

// portableDesktop implements Desktop by shelling out to the
// portabledesktop CLI via agentexec.Execer.
type portableDesktop struct {
	logger       slog.Logger
	execer       agentexec.Execer
	scriptBinDir string // coder script bin directory
	clock        quartz.Clock

	mu                  sync.Mutex
	session             *desktopSession // nil until started
	binPath             string          // resolved path to binary, cached
	closed              bool
	recordings          map[string]*recordingProcess // guarded by mu
	lastDesktopActionAt atomic.Int64
}

// NewPortableDesktop creates a Desktop backed by the portabledesktop
// CLI binary, using execer to spawn child processes. scriptBinDir is
// the coder script bin directory checked for the binary. If clk is
// nil, a real clock is used.
func NewPortableDesktop(
	logger slog.Logger,
	execer agentexec.Execer,
	scriptBinDir string,
	clk quartz.Clock,
) Desktop {
	if clk == nil {
		clk = quartz.NewReal()
	}
	pd := &portableDesktop{
		logger:       logger,
		execer:       execer,
		scriptBinDir: scriptBinDir,
		clock:        clk,
		recordings:   make(map[string]*recordingProcess),
	}
	pd.lastDesktopActionAt.Store(clk.Now().UnixNano())
	return pd
}

// Start launches the desktop session (idempotent).
func (p *portableDesktop) Start(ctx context.Context) (DisplayConfig, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return DisplayConfig{}, ErrDesktopClosed
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

// StartRecording begins recording the desktop to an MP4 file.
// Three-state idempotency: active recordings are no-ops,
// completed recordings are discarded and restarted.
func (p *portableDesktop) StartRecording(ctx context.Context, recordingID string) error {
	// Ensure the desktop session is running before acquiring the
	// recording lock. Start is independently locked and idempotent.
	if _, err := p.Start(ctx); err != nil {
		return xerrors.Errorf("ensure desktop session: %w", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrDesktopClosed
	}

	// Three-state idempotency:
	// - Active recording → no-op, continue recording.
	// - Completed recording → discard old file, start fresh.
	// - Unknown ID → fall through to start a new recording.
	if rec, ok := p.recordings[recordingID]; ok {
		if !rec.stopped {
			select {
			case <-rec.done:
				// Process exited unexpectedly; treat as completed
				// so we fall through to discard the old file and
				// restart.
			default:
				// Active recording - no-op, continue recording.
				return nil
			}
		}
		// Completed recording - discard old file, start fresh.
		if err := os.Remove(rec.filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			p.logger.Warn(ctx, "failed to remove old recording file",
				slog.F("recording_id", recordingID),
				slog.F("file_path", rec.filePath),
				slog.Error(err),
			)
		}
		if err := os.Remove(rec.thumbPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			p.logger.Warn(ctx, "failed to remove old thumbnail file",
				slog.F("recording_id", recordingID),
				slog.F("thumbnail_path", rec.thumbPath),
				slog.Error(err),
			)
		}
		delete(p.recordings, recordingID)
	}

	// Check concurrent recording limit.
	if p.lockedActiveRecordingCount() >= maxConcurrentRecordings {
		return xerrors.Errorf("too many concurrent recordings (max %d)", maxConcurrentRecordings)
	}

	// GC sweep: remove stopped recordings with stale files.
	p.lockedCleanStaleRecordings(ctx)

	if err := p.ensureBinary(ctx); err != nil {
		return xerrors.Errorf("ensure portabledesktop binary: %w", err)
	}

	filePath := filepath.Join(os.TempDir(), "coder-recording-"+recordingID+".mp4")
	thumbPath := filepath.Join(os.TempDir(), "coder-recording-"+recordingID+".thumb.jpg")

	// Use a background context so the process outlives the HTTP
	// request that triggered it.
	procCtx, procCancel := context.WithCancel(context.Background())

	//nolint:gosec // portabledesktop is a trusted binary resolved via ensureBinary.
	cmd := p.execer.CommandContext(procCtx, p.binPath, "record",
		// The following options are used to speed up the recording when the desktop is idle.
		// They were taken out of an example in the portabledesktop repo.
		// There's likely room for improvement to optimize the values.
		"--idle-speedup", "20",
		"--idle-min-duration", "0.35",
		"--idle-noise-tolerance", "-38dB",
		"--thumbnail", thumbPath,
		filePath)

	if err := cmd.Start(); err != nil {
		procCancel()
		return xerrors.Errorf("start recording process: %w", err)
	}

	rec := &recordingProcess{
		cmd:       cmd,
		filePath:  filePath,
		thumbPath: thumbPath,
		done:      make(chan struct{}),
	}
	go func() {
		rec.waitErr = cmd.Wait()
		close(rec.done)
		// avoid a context resource leak by canceling the context
		procCancel()
	}()

	p.recordings[recordingID] = rec

	p.logger.Info(ctx, "started desktop recording",
		slog.F("recording_id", recordingID),
		slog.F("file_path", filePath),
		slog.F("pid", cmd.Process.Pid),
	)

	// Record activity so a recording started on an already-idle
	// desktop does not stop immediately.
	p.lastDesktopActionAt.Store(p.clock.Now().UnixNano())

	// Spawn a per-recording idle goroutine.
	idleCtx, idleCancel := context.WithCancel(context.Background())
	rec.idleCancel = idleCancel
	rec.idleDone = make(chan struct{})
	go func() {
		defer close(rec.idleDone)
		p.monitorRecordingIdle(idleCtx, rec)
	}()

	return nil
}

// StopRecording finalizes the recording. Idempotent - safe to call
// on an already-stopped recording. Returns a RecordingArtifact
// that the caller can stream. The caller must close the Reader
// on the returned artifact to avoid leaking file descriptors.
func (p *portableDesktop) StopRecording(ctx context.Context, recordingID string) (*RecordingArtifact, error) {
	p.mu.Lock()
	rec, ok := p.recordings[recordingID]
	if !ok {
		p.mu.Unlock()
		return nil, ErrUnknownRecording
	}

	p.lockedStopRecordingProcess(ctx, rec, false)
	killed := rec.killed
	p.mu.Unlock()

	p.logger.Info(ctx, "stopped desktop recording",
		slog.F("recording_id", recordingID),
		slog.F("file_path", rec.filePath),
	)

	if killed {
		return nil, ErrRecordingCorrupted
	}

	// Open the file and return an artifact. Each call opens a fresh
	// file descriptor so the caller is insulated from restarts and
	// desktop close.
	f, err := os.Open(rec.filePath)
	if err != nil {
		return nil, xerrors.Errorf("open recording artifact: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, xerrors.Errorf("stat recording artifact: %w", err)
	}
	artifact := &RecordingArtifact{
		Reader: f,
		Size:   info.Size(),
	}
	// Attach thumbnail if the subprocess wrote one.
	thumbFile, err := os.Open(rec.thumbPath)
	if err != nil {
		p.logger.Warn(ctx, "thumbnail not available",
			slog.F("thumbnail_path", rec.thumbPath),
			slog.Error(err))
		return artifact, nil
	}
	thumbInfo, err := thumbFile.Stat()
	if err != nil {
		_ = thumbFile.Close()
		p.logger.Warn(ctx, "thumbnail stat failed",
			slog.F("thumbnail_path", rec.thumbPath),
			slog.Error(err))
		return artifact, nil
	}
	if thumbInfo.Size() == 0 {
		_ = thumbFile.Close()
		p.logger.Warn(ctx, "thumbnail file is empty",
			slog.F("thumbnail_path", rec.thumbPath))
		return artifact, nil
	}
	artifact.ThumbnailReader = thumbFile
	artifact.ThumbnailSize = thumbInfo.Size()
	return artifact, nil
}

// lockedStopRecordingProcess stops a single recording via stopOnce.
// It sends SIGINT, waits up to 15 seconds for graceful exit, then
// SIGKILLs. When force is true the process is SIGKILLed immediately
// without attempting a graceful shutdown. Must be called while p.mu
// is held; the lock is held for the full duration so that no
// concurrent StopRecording caller can read rec.stopped = true
// before the process has finished writing the MP4 file.
//
//nolint:revive // force flag keeps shared stopOnce/cleanup logic in one place.
func (p *portableDesktop) lockedStopRecordingProcess(ctx context.Context, rec *recordingProcess, force bool) {
	rec.stopOnce.Do(func() {
		if force {
			_ = rec.cmd.Process.Kill()
			rec.killed = true
		} else {
			_ = interruptRecordingProcess(rec.cmd.Process)
			timer := p.clock.NewTimer(15*time.Second, "agentdesktop", "stop_timeout")
			defer timer.Stop()
			select {
			case <-rec.done:
			case <-ctx.Done():
				_ = rec.cmd.Process.Kill()
				rec.killed = true
			case <-timer.C:
				_ = rec.cmd.Process.Kill()
				rec.killed = true
			}
		}
		rec.stopped = true
		if rec.idleCancel != nil {
			rec.idleCancel()
		}
	})
	// NOTE: We intentionally do not wait on rec.done here.
	// If goleak is added to this package's tests, this may
	// need revisiting to avoid flakes.
}

// lockedActiveRecordingCount returns the number of recordings that
// are still actively running. Must be called while p.mu is held.
// The max concurrency is low (maxConcurrentRecordings = 5), so a
// full scan is cheap and avoids maintaining a separate counter.
func (p *portableDesktop) lockedActiveRecordingCount() int {
	active := 0
	for _, rec := range p.recordings {
		if rec.stopped {
			continue
		}
		select {
		case <-rec.done:
		default:
			active++
		}
	}
	return active
}

// lockedCleanStaleRecordings removes stopped recordings whose temp
// files are older than one hour. Must be called while p.mu is held.
func (p *portableDesktop) lockedCleanStaleRecordings(ctx context.Context) {
	for id, rec := range p.recordings {
		if !rec.stopped {
			continue
		}
		info, err := os.Stat(rec.filePath)
		if err != nil {
			// File already removed or inaccessible; clean up
			// any leftover thumbnail and drop the entry.
			if err := os.Remove(rec.thumbPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				p.logger.Warn(ctx, "failed to remove stale thumbnail file",
					slog.F("recording_id", id),
					slog.F("thumbnail_path", rec.thumbPath),
					slog.Error(err),
				)
			}
			delete(p.recordings, id)
			continue
		}
		if p.clock.Since(info.ModTime()) > time.Hour {
			if err := os.Remove(rec.filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
				p.logger.Warn(ctx, "failed to remove stale recording file",
					slog.F("recording_id", id),
					slog.F("file_path", rec.filePath),
					slog.Error(err),
				)
			}
			if err := os.Remove(rec.thumbPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				p.logger.Warn(ctx, "failed to remove stale thumbnail file",
					slog.F("recording_id", id),
					slog.F("thumbnail_path", rec.thumbPath),
					slog.Error(err),
				)
			}
			delete(p.recordings, id)
		}
	}
}

// Close shuts down the desktop session and cleans up resources.
func (p *portableDesktop) Close() error {
	p.mu.Lock()
	p.closed = true

	// Force-kill all active recordings. The stopOnce inside
	// lockedStopRecordingProcess makes this safe for
	// already-stopped recordings.
	for _, rec := range p.recordings {
		p.lockedStopRecordingProcess(context.Background(), rec, true)
	}

	// Snapshot recording file paths and idle goroutine channels
	// for cleanup, then clear the map.
	type recEntry struct {
		id        string
		filePath  string
		thumbPath string
		idleDone  chan struct{}
	}
	var allRecs []recEntry
	for id, rec := range p.recordings {
		allRecs = append(allRecs, recEntry{id: id, filePath: rec.filePath, thumbPath: rec.thumbPath, idleDone: rec.idleDone})
		delete(p.recordings, id)
	}
	session := p.session
	p.session = nil
	p.mu.Unlock()

	// Wait for all per-recording idle goroutines to exit.
	for _, entry := range allRecs {
		if entry.idleDone != nil {
			<-entry.idleDone
		}
	}

	// Remove all recording files and wait for the session to
	// exit with a timeout so a slow filesystem or hung process
	// cannot block agent shutdown indefinitely.
	cleanupDone := make(chan struct{})
	go func() {
		defer close(cleanupDone)
		for _, entry := range allRecs {
			if err := os.Remove(entry.filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
				p.logger.Warn(context.Background(), "failed to remove recording file on close",
					slog.F("recording_id", entry.id),
					slog.F("file_path", entry.filePath),
					slog.Error(err),
				)
			}
			if err := os.Remove(entry.thumbPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				p.logger.Warn(context.Background(), "failed to remove thumbnail file on close",
					slog.F("recording_id", entry.id),
					slog.F("thumbnail_path", entry.thumbPath),
					slog.Error(err),
				)
			}
		}
		if session != nil {
			session.cancel()
			if err := session.cmd.Process.Kill(); err != nil {
				p.logger.Warn(context.Background(), "failed to kill portabledesktop process",
					slog.Error(err),
				)
			}
			if err := session.cmd.Wait(); err != nil {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					p.logger.Warn(context.Background(), "portabledesktop process exited with error",
						slog.Error(err),
					)
				}
			}
		}
	}()
	timer := p.clock.NewTimer(15*time.Second, "agentdesktop", "close_cleanup_timeout")
	defer timer.Stop()
	select {
	case <-cleanupDone:
	case <-timer.C:
		p.logger.Warn(context.Background(), "timed out waiting for close cleanup")
	}
	return nil
}

// RecordActivity marks the desktop as having received user
// interaction, resetting the idle-recording timer.
func (p *portableDesktop) RecordActivity() {
	p.lastDesktopActionAt.Store(p.clock.Now().UnixNano())
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

// monitorRecordingIdle watches for desktop inactivity and stops the
// given recording when the idle timeout is reached.
func (p *portableDesktop) monitorRecordingIdle(ctx context.Context, rec *recordingProcess) {
	timer := p.clock.NewTimer(idleTimeout, "agentdesktop", "recording_idle")
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			lastNano := p.lastDesktopActionAt.Load()
			lastAction := time.Unix(0, lastNano)
			elapsed := p.clock.Since(lastAction)
			if elapsed >= idleTimeout {
				p.mu.Lock()
				p.lockedStopRecordingProcess(context.Background(), rec, false)
				p.mu.Unlock()
				return
			}
			// Activity happened; reset with remaining budget.
			timer.Reset(idleTimeout-elapsed, "agentdesktop", "recording_idle")
		case <-rec.done:
			return
		case <-ctx.Done():
			return
		}
	}
}
