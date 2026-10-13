package agentbrowser

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/target"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/quartz"
	"github.com/coder/websocket"
)

//go:embed helper.html
var helperHTML []byte

// ErrBrowserNotFound is returned when no Chrome or Chromium binary is
// available in the workspace.
var ErrBrowserNotFound = xerrors.New("Chrome or Chromium is not installed in the workspace")

const (
	defaultWidth  = 1280
	defaultHeight = 800
	maxWidth      = 1920
	maxHeight     = 1200
	minSize       = 200

	captureTimeout = 15 * time.Second
	// launchTimeout is generous because Chrome's first start in a fresh
	// workspace builds its font cache, which took over 25s on CI runners.
	launchTimeout    = 2 * time.Minute
	viewerQueueSize  = 64
	inputQueueSize   = 256
	defaultIdleClose = 5 * time.Minute
)

var browserBinaries = []string{
	"google-chrome",
	"google-chrome-stable",
	"chromium",
	"chromium-browser",
}

// Options configures a Browser.
type Options struct {
	Logger       slog.Logger
	Execer       agentexec.Execer
	Clock        quartz.Clock
	ScriptBinDir string
	// ProfileDir holds the persistent Chrome profile. Defaults to a
	// directory under the user's config dir.
	ProfileDir string
	// IdleTimeout is how long Chrome keeps running without viewers.
	IdleTimeout time.Duration
}

// Browser runs a single headless Chrome tab inside the workspace and
// streams it, as video encoded by Chrome itself, to any number of viewers.
// Every viewer can drive input and navigation.
type Browser struct {
	opts Options

	// startMu serializes Chrome launches, which are slow.
	startMu sync.Mutex

	mu      sync.Mutex
	sess    *session
	viewers map[*viewer]struct{}
	idle    *quartz.Timer
	width   int
	height  int
	closed  bool
}

// New creates a Browser. Chrome is started lazily on the first Attach.
func New(opts Options) *Browser {
	if opts.Clock == nil {
		opts.Clock = quartz.NewReal()
	}
	if opts.Execer == nil {
		opts.Execer = agentexec.DefaultExecer
	}
	if opts.IdleTimeout == 0 {
		opts.IdleTimeout = defaultIdleClose
	}
	return &Browser{
		opts:    opts,
		viewers: make(map[*viewer]struct{}),
		width:   defaultWidth,
		height:  defaultHeight,
	}
}

type viewer struct {
	send      chan []byte
	needKey   bool
	closed    chan struct{}
	closeOnce sync.Once
}

func (v *viewer) close() {
	v.closeOnce.Do(func() { close(v.closed) })
}

// trySend queues a frame without blocking and reports whether it fit.
func (v *viewer) trySend(frame []byte) bool {
	select {
	case v.send <- frame:
		return true
	default:
		return false
	}
}

// attach starts Chrome if needed and registers a new viewer.
func (b *Browser) attach(ctx context.Context) (*viewer, error) {
	sess, err := b.ensureSession(ctx)
	if err != nil {
		return nil, err
	}

	v := &viewer{
		send:    make(chan []byte, viewerQueueSize),
		needKey: true,
		closed:  make(chan struct{}),
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.sess != sess {
		return nil, xerrors.New("browser stopped while attaching")
	}
	b.viewers[v] = struct{}{}
	if b.idle != nil {
		b.idle.Stop()
		b.idle = nil
	}
	if cfg := sess.config.Load(); cfg != nil {
		v.trySend(*cfg)
	}
	if st := sess.lastState.Load(); st != nil {
		v.trySend(encodeJSONFrame(st))
	}
	sess.requestKey()
	return v, nil
}

// detach unregisters a viewer and schedules Chrome to stop once no
// viewers remain.
func (b *Browser) detach(v *viewer) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.viewers, v)
	v.close()
	if len(b.viewers) == 0 && b.sess != nil && b.idle == nil && !b.closed {
		sess := b.sess
		b.idle = b.opts.Clock.AfterFunc(b.opts.IdleTimeout, func() {
			b.mu.Lock()
			if b.sess != sess || len(b.viewers) > 0 {
				b.mu.Unlock()
				return
			}
			b.sess = nil
			b.idle = nil
			b.mu.Unlock()
			b.opts.Logger.Info(context.Background(), "stopping idle browser")
			sess.close()
		}, "agentbrowser", "idle")
	}
}

// handleMessage applies a message from a viewer.
func (b *Browser) handleMessage(v *viewer, msg ClientMessage) {
	b.mu.Lock()
	sess := b.sess
	if msg.Type == "resize" {
		b.width, b.height = clampSize(msg.Width, msg.Height)
		msg.Width, msg.Height = b.width, b.height
	}
	b.mu.Unlock()
	if sess == nil {
		return
	}

	switch msg.Type {
	case "keyframe":
		sess.requestKey()
		return
	case "navigate":
		u, err := normalizeURL(msg.URL)
		if err != nil {
			v.trySend(encodeJSONFrame(ServerError{Type: "error", Message: err.Error()}))
			return
		}
		msg.URL = u
	}
	select {
	case sess.input <- msg:
	default:
		// Input is lossy under extreme backlog rather than unbounded.
	}
}

// Close stops Chrome and disconnects all viewers.
func (b *Browser) Close() error {
	b.mu.Lock()
	b.closed = true
	sess := b.sess
	b.sess = nil
	for v := range b.viewers {
		v.close()
		delete(b.viewers, v)
	}
	if b.idle != nil {
		b.idle.Stop()
		b.idle = nil
	}
	b.mu.Unlock()
	if sess != nil {
		sess.close()
	}
	return nil
}

func (b *Browser) ensureSession(ctx context.Context) (*session, error) {
	b.startMu.Lock()
	defer b.startMu.Unlock()

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, xerrors.New("browser closed")
	}
	if b.sess != nil {
		sess := b.sess
		b.mu.Unlock()
		return sess, nil
	}
	width, height := b.width, b.height
	b.mu.Unlock()

	sess, err := b.launch(ctx, width, height)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		go sess.close()
		return nil, xerrors.New("browser closed")
	}
	b.sess = sess
	return sess, nil
}

// broadcastVideo sends an encoded chunk ([flags][data]) to every viewer.
// Deltas are skipped for viewers waiting on a keyframe, and a slow viewer
// that overflows its queue waits for the next keyframe instead of
// stalling everyone.
func (b *Browser) broadcastVideo(sess *session, chunk []byte) {
	isKey := chunk[0]&videoFlagKey != 0
	frame := encodeFrame(frameTypeVideo, chunk)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sess != sess {
		return
	}
	needKey := false
	for v := range b.viewers {
		if v.needKey && !isKey {
			continue
		}
		v.needKey = false
		if !v.trySend(frame) {
			v.needKey = true
			needKey = true
		}
	}
	if needKey {
		sess.requestKey()
	}
}

// broadcastMessage sends a control frame to every viewer.
func (b *Browser) broadcastMessage(sess *session, frame []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sess != sess {
		return
	}
	for v := range b.viewers {
		v.trySend(frame)
	}
}

// markAllNeedKey makes every viewer wait for a keyframe, used when the
// encoder is reconfigured.
func (b *Browser) markAllNeedKey(sess *session) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sess != sess {
		return
	}
	for v := range b.viewers {
		v.needKey = true
	}
}

func (b *Browser) sessionEnded(sess *session, reason string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sess != sess {
		return
	}
	b.sess = nil
	frame := encodeJSONFrame(ServerError{Type: "error", Message: reason})
	for v := range b.viewers {
		v.trySend(frame)
		v.close()
		delete(b.viewers, v)
	}
}

func clampSize(w, h int) (width, height int) {
	width = min(max(w, minSize), maxWidth) &^ 1
	height = min(max(h, minSize), maxHeight) &^ 1
	return width, height
}

func (b *Browser) findBinary() (string, error) {
	if p := os.Getenv("CODER_AGENT_BROWSER_PATH"); p != "" {
		return p, nil
	}
	for _, name := range browserBinaries {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
		if b.opts.ScriptBinDir != "" {
			p := filepath.Join(b.opts.ScriptBinDir, name)
			if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
				return p, nil
			}
		}
	}
	return "", ErrBrowserNotFound
}

// logIfErr records best-effort failures that need no other handling.
func (s *session) logIfErr(msg string, err error) {
	if err != nil {
		s.logger.Debug(s.ctx, msg, slog.Error(err))
	}
}

// session is one running Chrome process with a content tab and a helper
// tab that captures and encodes the content tab.
type session struct {
	b      *Browser
	logger slog.Logger
	ctx    context.Context
	cancel context.CancelFunc

	cmd *exec.Cmd
	cdp *cdpConn
	srv *http.Server

	nonce     string
	token     string
	helperURL string

	// contentTarget and contentSession are written once during setup,
	// before contentReady is closed.
	contentTarget  target.ID
	contentSession string
	contentReady   chan struct{}
	mainFrame      atomic.Value // cdp.FrameID
	loading        atomic.Bool

	helperMu      sync.Mutex
	helperTarget  target.ID
	helperSession string
	helperWS      *websocket.Conn
	restarting    atomic.Bool

	popupsMu sync.Mutex
	popups   map[target.ID]struct{}

	config    atomic.Pointer[[]byte]
	lastState atomic.Pointer[ServerState]

	input     chan ClientMessage
	closeOnce sync.Once
}

func (b *Browser) launch(ctx context.Context, width, height int) (*session, error) {
	if runtime.GOOS == "windows" {
		return nil, xerrors.New("the workspace browser is not supported on Windows agents")
	}
	bin, err := b.findBinary()
	if err != nil {
		return nil, err
	}
	profileDir := b.opts.ProfileDir
	if profileDir == "" {
		cfgDir, err := os.UserConfigDir()
		if err != nil {
			return nil, xerrors.Errorf("find config dir: %w", err)
		}
		profileDir = filepath.Join(cfgDir, "coder-agent", "browser-profile")
	}
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return nil, xerrors.Errorf("create browser profile dir: %w", err)
	}

	sessCtx, cancel := context.WithCancel(context.Background())
	s := &session{
		b:            b,
		logger:       b.opts.Logger,
		ctx:          sessCtx,
		cancel:       cancel,
		nonce:        "coder-browser-" + rand.Text(),
		token:        rand.Text(),
		popups:       make(map[target.ID]struct{}),
		input:        make(chan ClientMessage, inputQueueSize),
		contentReady: make(chan struct{}),
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		cancel()
		return nil, xerrors.Errorf("listen for helper: %w", err)
	}
	s.helperURL = "http://" + ln.Addr().String() + "/helper?token=" + s.token
	mux := http.NewServeMux()
	mux.HandleFunc("/helper", s.serveHelperPage)
	mux.HandleFunc("/ws", s.serveHelperWS)
	s.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := s.srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
			s.logIfErr("browser helper server stopped", err)
		}
	}()

	// Chrome reads commands from fd 3 and writes responses to fd 4.
	cmdR, cmdW, err := os.Pipe()
	if err != nil {
		s.close()
		return nil, xerrors.Errorf("create pipe: %w", err)
	}
	respR, respW, err := os.Pipe()
	if err != nil {
		_ = cmdR.Close()
		_ = cmdW.Close()
		s.close()
		return nil, xerrors.Errorf("create pipe: %w", err)
	}

	args := []string{
		"--headless=new",
		"--remote-debugging-pipe",
		"--user-data-dir=" + profileDir,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-dev-shm-usage",
		"--disable-background-timer-throttling",
		"--disable-renderer-backgrounding",
		"--disable-backgrounding-occluded-windows",
		"--mute-audio",
		"--window-size=1920,1200",
		"--auto-select-tab-capture-source-by-title=" + s.nonce,
	}
	if os.Geteuid() == 0 {
		b.opts.Logger.Warn(ctx, "running Chrome as root requires disabling its sandbox")
		args = append(args, "--no-sandbox")
	}
	args = append(args, "about:blank")

	s.cmd = b.opts.Execer.CommandContext(sessCtx, bin, args...)
	s.cmd.ExtraFiles = []*os.File{cmdR, respW}
	s.cmd.Stderr = &logWriter{logger: b.opts.Logger}
	if err := s.cmd.Start(); err != nil {
		_ = cmdR.Close()
		_ = cmdW.Close()
		_ = respR.Close()
		_ = respW.Close()
		s.close()
		return nil, xerrors.Errorf("start %s: %w", bin, err)
	}
	_ = cmdR.Close()
	_ = respW.Close()
	b.opts.Logger.Info(ctx, "started workspace browser", slog.F("path", bin), slog.F("pid", s.cmd.Process.Pid))

	s.cdp = newCDPConn(cmdW, respR, s.handleEvent)
	go func() {
		err := s.cmd.Wait()
		b.opts.Logger.Info(context.Background(), "workspace browser exited", slog.Error(err))
		b.sessionEnded(s, "The browser exited.")
		s.close()
	}()
	go s.inputLoop()

	setupCtx, setupCancel := context.WithTimeout(ctx, launchTimeout)
	defer setupCancel()
	if err := s.setup(setupCtx, width, height); err != nil {
		s.close()
		return nil, xerrors.Errorf("set up browser: %w", err)
	}
	if err := s.startCapture(setupCtx, width, height); err != nil {
		s.close()
		return nil, xerrors.Errorf("start capture: %w", err)
	}
	return s, nil
}

func (s *session) close() {
	s.closeOnce.Do(func() {
		s.cancel()
		if s.cdp != nil {
			closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := s.cdp.Call(closeCtx, "", "Browser.close", nil, nil)
			cancel()
			s.logIfErr("close browser over cdp", err)
			_ = s.cdp.Close()
		}
		if s.srv != nil {
			_ = s.srv.Close()
		}
		if s.cmd != nil && s.cmd.Process != nil {
			if err := s.cmd.Process.Kill(); !errors.Is(err, os.ErrProcessDone) {
				s.logIfErr("kill browser process", err)
			}
		}
	})
}

func (s *session) call(ctx context.Context, sessionID, method string, params, result any) error {
	return s.cdp.Call(ctx, sessionID, method, params, result)
}

func (s *session) setup(ctx context.Context, width, height int) error {
	if err := s.call(ctx, "", "Target.setDiscoverTargets", map[string]any{"discover": true}, nil); err != nil {
		return err
	}
	if err := s.call(ctx, "", "Browser.setDownloadBehavior", map[string]any{"behavior": "deny"}, nil); err != nil {
		return err
	}

	var targets struct {
		TargetInfos []*target.Info `json:"targetInfos"`
	}
	if err := s.call(ctx, "", "Target.getTargets", nil, &targets); err != nil {
		return err
	}
	for _, t := range targets.TargetInfos {
		if t.Type == "page" {
			s.contentTarget = t.TargetID
			break
		}
	}
	if s.contentTarget == "" {
		var created target.CreateTargetReturns
		if err := s.call(ctx, "", "Target.createTarget", target.CreateTarget("about:blank"), &created); err != nil {
			return err
		}
		s.contentTarget = created.TargetID
	}
	sessionID, err := s.attach(ctx, s.contentTarget)
	if err != nil {
		return err
	}
	s.contentSession = sessionID
	close(s.contentReady)

	steps := []struct {
		method string
		params any
	}{
		{"Page.enable", nil},
		{"Page.setInterceptFileChooserDialog", page.SetInterceptFileChooserDialog(true).WithCancel(true)},
		{"Emulation.setFocusEmulationEnabled", emulation.SetFocusEmulationEnabled(true)},
		{"Emulation.setDeviceMetricsOverride", emulation.SetDeviceMetricsOverride(int64(width), int64(height), 1, false)},
	}
	for _, step := range steps {
		if err := s.call(ctx, s.contentSession, step.method, step.params, nil); err != nil {
			return err
		}
	}
	var tree page.GetFrameTreeReturns
	if err := s.call(ctx, s.contentSession, "Page.getFrameTree", nil, &tree); err != nil {
		return err
	}
	if tree.FrameTree != nil && tree.FrameTree.Frame != nil {
		s.mainFrame.Store(tree.FrameTree.Frame.ID)
	}
	s.refreshState(ctx)
	return nil
}

func (s *session) attach(ctx context.Context, id target.ID) (string, error) {
	var res target.AttachToTargetReturns
	if err := s.call(ctx, "", "Target.attachToTarget", target.AttachToTarget(id).WithFlatten(true), &res); err != nil {
		return "", err
	}
	return string(res.SessionID), nil
}

func (s *session) evaluate(ctx context.Context, sessionID, expr string, gesture bool) (json.RawMessage, error) {
	params := cdpruntime.Evaluate(expr).WithReturnByValue(true).WithAwaitPromise(true).WithUserGesture(gesture)
	var res cdpruntime.EvaluateReturns
	if err := s.call(ctx, sessionID, "Runtime.evaluate", params, &res); err != nil {
		return nil, err
	}
	if res.ExceptionDetails != nil {
		msg := res.ExceptionDetails.Text
		if res.ExceptionDetails.Exception != nil && res.ExceptionDetails.Exception.Description != "" {
			msg = res.ExceptionDetails.Exception.Description
		}
		return nil, xerrors.Errorf("evaluate: %s", msg)
	}
	if res.Result == nil {
		return nil, nil
	}
	return json.RawMessage(res.Result.Value), nil
}

// startCapture (re)creates the helper tab and starts capturing the
// content tab. Chrome's auto-select flag picks the tab whose title
// matches the per-launch nonce, so the content tab carries the nonce as
// its title for the duration of the getDisplayMedia call.
func (s *session) startCapture(ctx context.Context, width, height int) error {
	ctx, cancel := context.WithTimeout(ctx, captureTimeout)
	defer cancel()

	s.helperMu.Lock()
	oldTarget := s.helperTarget
	s.helperTarget = ""
	s.helperSession = ""
	// Detach the old helper connection first so its close isn't treated
	// as a failure that triggers another restart.
	oldWS := s.helperWS
	s.helperWS = nil
	s.helperMu.Unlock()
	if oldWS != nil {
		s.logIfErr("close old helper websocket", oldWS.CloseNow())
	}
	if oldTarget != "" {
		s.logIfErr("close old helper tab", s.call(ctx, "", "Target.closeTarget", target.CloseTarget(oldTarget), nil))
	}

	var created target.CreateTargetReturns
	if err := s.call(ctx, "", "Target.createTarget", target.CreateTarget(s.helperURL), &created); err != nil {
		return err
	}
	helperSession, err := s.attach(ctx, created.TargetID)
	if err != nil {
		return err
	}
	s.helperMu.Lock()
	s.helperTarget = created.TargetID
	s.helperSession = helperSession
	s.helperMu.Unlock()

	for {
		v, err := s.evaluate(ctx, helperSession, `typeof window.coderStart`, false)
		if err == nil && string(v) == `"function"` {
			break
		}
		select {
		case <-ctx.Done():
			return xerrors.Errorf("helper page did not load: %w", ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}

	nonceJSON, err := json.Marshal(s.nonce)
	if err != nil {
		return xerrors.Errorf("marshal nonce: %w", err)
	}
	oldTitle, err := s.evaluate(ctx, s.contentSession,
		`(() => { const t = document.title; document.title = `+string(nonceJSON)+`; return t; })()`, false)
	if err != nil {
		return xerrors.Errorf("tag content tab: %w", err)
	}
	if len(oldTitle) == 0 {
		oldTitle = json.RawMessage(`""`)
	}
	defer func() {
		restoreCtx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		defer cancel()
		_, err := s.evaluate(restoreCtx, s.contentSession,
			`if (document.title === `+string(nonceJSON)+`) document.title = `+string(oldTitle), false)
		s.logIfErr("restore content tab title", err)
	}()

	res, err := s.evaluate(ctx, helperSession,
		"coderStart("+strconv.Itoa(width)+","+strconv.Itoa(height)+")", true)
	if err != nil {
		return err
	}
	s.logger.Info(ctx, "browser capture started", slog.F("result", string(res)))
	return nil
}

// restartCapture recovers from a helper or capture failure. If capture
// cannot be restarted, Chrome is stopped so the next viewer starts fresh.
func (s *session) restartCapture(reason string) {
	if s.ctx.Err() != nil || !s.restarting.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer s.restarting.Store(false)
		s.logger.Warn(s.ctx, "restarting browser capture", slog.F("reason", reason))
		for attempt := range 3 {
			s.b.mu.Lock()
			width, height := s.b.width, s.b.height
			s.b.mu.Unlock()
			err := s.startCapture(s.ctx, width, height)
			if err == nil {
				return
			}
			s.logger.Warn(s.ctx, "restart capture failed", slog.F("attempt", attempt), slog.Error(err))
			if s.ctx.Err() != nil {
				return
			}
		}
		s.b.sessionEnded(s, "Video capture failed. Reconnect to restart the browser.")
		s.close()
	}()
}

func (s *session) serveHelperPage(rw http.ResponseWriter, r *http.Request) {
	if !s.validToken(r) {
		http.Error(rw, "forbidden", http.StatusForbidden)
		return
	}
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err := rw.Write(helperHTML)
	s.logIfErr("write helper page", err)
}

func (s *session) validToken(r *http.Request) bool {
	return subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("token")), []byte(s.token)) == 1
}

type helperMessage struct {
	Type    string `json:"type"`
	Codec   string `json:"codec"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Message string `json:"message"`
}

func (s *session) serveHelperWS(rw http.ResponseWriter, r *http.Request) {
	if !s.validToken(r) {
		http.Error(rw, "forbidden", http.StatusForbidden)
		return
	}
	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	conn.SetReadLimit(64 << 20)
	s.helperMu.Lock()
	old := s.helperWS
	s.helperWS = conn
	s.helperMu.Unlock()
	if old != nil {
		s.logIfErr("close replaced helper websocket", old.CloseNow())
	}

	defer func() {
		s.helperMu.Lock()
		replaced := s.helperWS != conn
		if !replaced {
			s.helperWS = nil
		}
		s.helperMu.Unlock()
		s.logIfErr("close helper websocket", conn.CloseNow())
		if !replaced {
			s.restartCapture("helper disconnected")
		}
	}()

	for {
		typ, data, err := conn.Read(s.ctx)
		if err != nil {
			return
		}
		if typ == websocket.MessageBinary {
			if len(data) < 2 {
				continue
			}
			s.b.broadcastVideo(s, data)
			continue
		}
		var msg helperMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "config":
			frame := encodeJSONFrame(ServerConfig{Type: "config", Codec: msg.Codec, Width: msg.Width, Height: msg.Height})
			s.config.Store(&frame)
			s.b.markAllNeedKey(s)
			s.b.broadcastMessage(s, frame)
		case "error":
			s.logger.Warn(s.ctx, "browser capture error", slog.F("message", msg.Message))
		}
	}
}

func (s *session) sendHelper(msg map[string]any) {
	s.helperMu.Lock()
	conn := s.helperWS
	s.helperMu.Unlock()
	if conn == nil {
		return
	}
	data, err := json.Marshal(msg)
	if err != nil {
		s.logIfErr("marshal helper message", err)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		defer cancel()
		s.logIfErr("send helper message", conn.Write(ctx, websocket.MessageText, data))
	}()
}

func (s *session) requestKey() {
	s.sendHelper(map[string]any{"type": "key"})
}

// inputLoop applies viewer messages in order.
func (s *session) inputLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case msg := <-s.input:
			ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
			if err := s.apply(ctx, msg); err != nil {
				s.logger.Debug(ctx, "apply browser input", slog.F("type", msg.Type), slog.Error(err))
			}
			cancel()
		}
	}
}

func (s *session) apply(ctx context.Context, msg ClientMessage) error {
	switch msg.Type {
	case "mouse":
		var typ input.MouseType
		switch msg.Action {
		case "down":
			typ = input.MousePressed
		case "up":
			typ = input.MouseReleased
		case "move":
			typ = input.MouseMoved
		case "wheel":
			typ = input.MouseWheel
		default:
			return xerrors.Errorf("unknown mouse action %q", msg.Action)
		}
		params := &input.DispatchMouseEventParams{
			Type:       typ,
			X:          msg.X,
			Y:          msg.Y,
			Modifiers:  input.Modifier(msg.Modifiers),
			Button:     mouseButton(msg.Button),
			Buttons:    msg.Buttons,
			ClickCount: msg.ClickCount,
			DeltaX:     msg.DeltaX,
			DeltaY:     msg.DeltaY,
		}
		return s.call(ctx, s.contentSession, "Input.dispatchMouseEvent", params, nil)
	case "key":
		params := &input.DispatchKeyEventParams{
			Modifiers:             input.Modifier(msg.Modifiers),
			Key:                   msg.Key,
			Code:                  msg.Code,
			WindowsVirtualKeyCode: msg.KeyCode,
			NativeVirtualKeyCode:  msg.KeyCode,
		}
		switch {
		case msg.Action == "up":
			params.Type = input.KeyUp
		case msg.Text != "":
			params.Type = input.KeyDown
			params.Text = msg.Text
			params.UnmodifiedText = msg.Text
		default:
			params.Type = input.KeyRawDown
		}
		return s.call(ctx, s.contentSession, "Input.dispatchKeyEvent", params, nil)
	case "insert_text":
		return s.call(ctx, s.contentSession, "Input.insertText", input.InsertText(msg.Text), nil)
	case "resize":
		if err := s.call(ctx, s.contentSession, "Emulation.setDeviceMetricsOverride",
			emulation.SetDeviceMetricsOverride(int64(msg.Width), int64(msg.Height), 1, false), nil); err != nil {
			return err
		}
		s.sendHelper(map[string]any{"type": "size", "width": msg.Width, "height": msg.Height})
		return nil
	case "navigate":
		return s.call(ctx, s.contentSession, "Page.navigate", page.Navigate(msg.URL), nil)
	case "reload":
		return s.call(ctx, s.contentSession, "Page.reload", page.Reload(), nil)
	case "back", "forward":
		var hist page.GetNavigationHistoryReturns
		if err := s.call(ctx, s.contentSession, "Page.getNavigationHistory", nil, &hist); err != nil {
			return err
		}
		idx := hist.CurrentIndex - 1
		if msg.Type == "forward" {
			idx = hist.CurrentIndex + 1
		}
		if idx < 0 || idx >= int64(len(hist.Entries)) {
			return nil
		}
		return s.call(ctx, s.contentSession, "Page.navigateToHistoryEntry",
			page.NavigateToHistoryEntry(hist.Entries[idx].ID), nil)
	default:
		return xerrors.Errorf("unknown message type %q", msg.Type)
	}
}

func mouseButton(b string) input.MouseButton {
	switch b {
	case "left", "middle", "right", "back", "forward":
		return input.MouseButton(b)
	default:
		return input.None
	}
}

func (s *session) isMainFrame(id cdp.FrameID) bool {
	main, _ := s.mainFrame.Load().(cdp.FrameID)
	return main == "" || main == id
}

// handleEvent runs on the CDP event goroutine, in order.
func (s *session) handleEvent(msg cdpMessage) {
	select {
	case <-s.contentReady:
	case <-s.ctx.Done():
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	switch msg.Method {
	case "Target.targetCreated", "Target.targetInfoChanged":
		var ev target.EventTargetInfoChanged
		if json.Unmarshal(msg.Params, &ev) != nil || ev.TargetInfo == nil {
			return
		}
		info := ev.TargetInfo
		if info.TargetID == s.contentTarget {
			s.refreshState(ctx)
			return
		}
		s.popupsMu.Lock()
		_, isPopup := s.popups[info.TargetID]
		if !isPopup && info.Type == "page" && info.OpenerID == s.contentTarget {
			s.popups[info.TargetID] = struct{}{}
			isPopup = true
		}
		s.popupsMu.Unlock()
		if isPopup && info.URL != "" && info.URL != "about:blank" {
			s.redirectPopup(ctx, info)
		}
	case "Target.targetDestroyed":
		var ev target.EventTargetDestroyed
		if json.Unmarshal(msg.Params, &ev) == nil {
			s.popupsMu.Lock()
			delete(s.popups, ev.TargetID)
			s.popupsMu.Unlock()
		}
	}

	if msg.SessionID == "" || msg.SessionID != s.contentSession {
		return
	}
	switch msg.Method {
	case "Page.frameNavigated":
		var ev page.EventFrameNavigated
		if json.Unmarshal(msg.Params, &ev) == nil && ev.Frame != nil && ev.Frame.ParentID == "" {
			s.mainFrame.Store(ev.Frame.ID)
			s.refreshState(ctx)
		}
	case "Page.navigatedWithinDocument":
		s.refreshState(ctx)
	case "Page.frameStartedLoading":
		var ev page.EventFrameStartedLoading
		if json.Unmarshal(msg.Params, &ev) == nil && s.isMainFrame(ev.FrameID) {
			s.loading.Store(true)
			s.refreshState(ctx)
		}
	case "Page.frameStoppedLoading":
		var ev page.EventFrameStoppedLoading
		if json.Unmarshal(msg.Params, &ev) == nil && s.isMainFrame(ev.FrameID) {
			s.loading.Store(false)
			s.refreshState(ctx)
		}
	case "Page.javascriptDialogOpening":
		// There is no dialog UI, so dismiss dialogs to keep pages from
		// hanging. Alerts and beforeunload are accepted.
		var ev page.EventJavascriptDialogOpening
		if json.Unmarshal(msg.Params, &ev) != nil {
			return
		}
		accept := ev.Type == page.DialogTypeAlert || ev.Type == page.DialogTypeBeforeunload
		s.logIfErr("dismiss javascript dialog",
			s.call(ctx, s.contentSession, "Page.handleJavaScriptDialog", page.HandleJavaScriptDialog(accept), nil))
	}
}

// redirectPopup keeps the browser single-tab: new windows are closed and
// their URL opens in the content tab instead.
func (s *session) redirectPopup(ctx context.Context, info *target.Info) {
	s.popupsMu.Lock()
	delete(s.popups, info.TargetID)
	s.popupsMu.Unlock()
	s.logIfErr("close popup tab", s.call(ctx, "", "Target.closeTarget", target.CloseTarget(info.TargetID), nil))
	if u, err := normalizeURL(info.URL); err == nil {
		s.logIfErr("open popup url", s.call(ctx, s.contentSession, "Page.navigate", page.Navigate(u), nil))
	}
}

func (s *session) refreshState(ctx context.Context) {
	var hist page.GetNavigationHistoryReturns
	if err := s.call(ctx, s.contentSession, "Page.getNavigationHistory", nil, &hist); err != nil {
		return
	}
	if hist.CurrentIndex < 0 || hist.CurrentIndex >= int64(len(hist.Entries)) {
		return
	}
	entry := hist.Entries[hist.CurrentIndex]
	st := ServerState{
		Type:         "state",
		URL:          entry.URL,
		Title:        entry.Title,
		Loading:      s.loading.Load(),
		CanGoBack:    hist.CurrentIndex > 0,
		CanGoForward: hist.CurrentIndex < int64(len(hist.Entries))-1,
	}
	prev := s.lastState.Load()
	if strings.HasPrefix(st.Title, "coder-browser-") {
		// The nonce title is only set while capture starts.
		if prev != nil {
			st.Title = prev.Title
		} else {
			st.Title = ""
		}
	}
	if prev != nil && *prev == st {
		return
	}
	s.lastState.Store(&st)
	s.b.broadcastMessage(s, encodeJSONFrame(st))
}

// logWriter forwards Chrome's stderr to the agent log at debug level.
type logWriter struct {
	logger slog.Logger
}

func (w *logWriter) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if line != "" {
			w.logger.Debug(context.Background(), "chrome", slog.F("line", line))
		}
	}
	return len(p), nil
}

var _ io.Writer = (*logWriter)(nil)
