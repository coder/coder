package agentnative

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/quartz"

	"github.com/coder/coder/v2/agent/agentexec"
)

const openCodeMarker = "OPENCODE_AGENT_INSTANCE"

type openCodeDirectoryMode uint8

const (
	openCodeExistingDirectory openCodeDirectoryMode = iota
	openCodeCreateDirectory
)

var errOpenCodeOwnership = xerrors.New("OpenCode process ownership could not be verified; private state was not repaired")

type openCodeDescriptor struct {
	Format    int    `json:"format"`
	Version   string `json:"version"`
	URL       string `json:"url"`
	Password  string `json:"password"`
	Marker    string `json:"marker"`
	Domain    string `json:"domain"`
	Status    string `json:"status"`
	PID       int    `json:"pid"`
	StartTime string `json:"startTime"`
}

type openCodeProcess struct {
	StartTime string
	Marker    string
	Ports     []int
}

type openCodeServer struct {
	options BackendOptions
	clock   quartz.Clock
	http    *http.Client
	// Seams are deliberately private: production always proves kernel ownership.
	domain  func() (string, error)
	process func(int) (*openCodeProcess, error)
	signal  func(openCodeDescriptor) error
	version func(context.Context) (string, error)
}

func newOpenCodeServer(options BackendOptions, clock quartz.Clock) *openCodeServer {
	s := &openCodeServer{options: options, clock: clock, http: openCodeHTTPClient(), domain: openCodeProcessDomain, process: openCodeProcessIdentity, signal: openCodeSignal}
	s.version = s.checkVersion
	return s
}

func (s *openCodeServer) descriptorPath() string {
	return filepath.Join(s.options.StateDir, "opencode-server.json")
}
func (s *openCodeServer) home() string { return filepath.Join(s.options.StateDir, "opencode-home") }
func (s *openCodeServer) environment() []string {
	return openCodeEnvironment(s.home(), s.options.Environment)
}

func openCodeEnvironment(home string, original []string) []string {
	var result []string
	for _, entry := range original {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if strings.HasPrefix(key, "OPENCODE_") {
			if key == "OPENCODE_DISABLE_MODELS_FETCH" && value == "true" {
				result = append(result, entry)
			}
			continue
		}
		switch key {
		case "HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "TMPDIR":
			continue
		}
		result = append(result, entry)
	}
	return append(result, "HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, "config"), "XDG_DATA_HOME="+filepath.Join(home, "data"), "XDG_STATE_HOME="+filepath.Join(home, "state"), "XDG_CACHE_HOME="+filepath.Join(home, "cache"), "TMPDIR="+filepath.Join(home, "tmp"), "OPENCODE_DISABLE_AUTOUPDATE=true", `OPENCODE_CONFIG_CONTENT={"update":"disable"}`)
}

func openCodeMakeHome(home string) error {
	for _, name := range []string{"", "config", "data", "state", "cache", "tmp"} {
		if err := openCodePrivateDirectory(filepath.Join(home, name), openCodeCreateDirectory); err != nil {
			return err
		}
	}
	return nil
}

func (s *openCodeServer) command(ctx context.Context, path string, args ...string) *exec.Cmd {
	if s.options.Command != nil {
		return s.options.Command(ctx, path, args...)
	}
	return agentexec.DefaultExecer.CommandContext(ctx, path, args...)
}

func (s *openCodeServer) binary() (string, error) {
	if s.options.Command != nil {
		return "opencode", nil
	}
	var path string
	for _, entry := range s.options.Environment {
		if strings.HasPrefix(entry, "PATH=") {
			path = strings.TrimPrefix(entry, "PATH=")
		}
	}
	for _, directory := range filepath.SplitList(path) {
		if !filepath.IsAbs(directory) {
			continue
		}
		candidate := filepath.Join(directory, "opencode")
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", xerrors.New("OpenCode CLI not found in workspace PATH")
}

type openCodeBoundedBuffer struct{ buffer bytes.Buffer }

func (b *openCodeBoundedBuffer) Write(data []byte) (int, error) {
	if len(data)+b.buffer.Len() > 1024 {
		return 0, xerrors.New("OpenCode CLI response exceeded limit")
	}
	return b.buffer.Write(data)
}

func (s *openCodeServer) checkVersion(ctx context.Context) (string, error) {
	path, err := s.binary()
	if err != nil {
		return "", err
	}
	// Never let an incompatible CLI initialize the persistent native database.
	home, err := os.MkdirTemp("", "coder-opencode-version-")
	if err != nil {
		return "", xerrors.New("OpenCode CLI version probe unavailable")
	}
	defer os.RemoveAll(home)
	if err := openCodeMakeHome(home); err != nil {
		return "", err
	}
	ctx, cancel := openCodeTimeout(ctx, s.clock, 3*time.Second)
	defer cancel()
	command := s.command(ctx, path, "--version")
	command.Dir = home
	command.Env = openCodeEnvironment(home, s.options.Environment)
	var output openCodeBoundedBuffer
	command.Stdout = &output
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return "", xerrors.New("OpenCode CLI version check failed")
	}
	if strings.TrimSpace(output.buffer.String()) != "opencode v"+openCodeVersion {
		return "", xerrors.New("OpenCode CLI must be exactly version 2.0.28")
	}
	return path, nil
}

func (s *openCodeServer) readDescriptor() (*openCodeDescriptor, error) {
	file, err := openCodePrivateFile(s.descriptorPath(), os.O_RDONLY)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil //nolint:nilnil // No descriptor is a valid state before first startup.
	}
	if err != nil {
		return nil, errOpenCodeOwnership
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 8193))
	if err != nil || len(data) > 8192 {
		return nil, errOpenCodeOwnership
	}
	var d openCodeDescriptor
	if json.Unmarshal(data, &d) != nil || d.Format != 1 || d.Version != openCodeVersion || !openCodeEndpoint(d.URL) || !openCodeDigest.MatchString(d.Password) || !openCodeDigest.MatchString(d.Marker) || d.Domain == "" || len(d.Domain) > 256 || (d.Status != "starting" && d.Status != "ready") {
		return nil, errOpenCodeOwnership
	}
	u, _ := url.Parse(d.URL)
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return nil, errOpenCodeOwnership
	}
	if d.PID == 0 && d.StartTime == "" && d.Status == "starting" {
		return &d, nil
	}
	start, err := strconv.ParseUint(d.StartTime, 10, 64)
	if d.PID <= 0 || err != nil || start == 0 {
		return nil, errOpenCodeOwnership
	}
	return &d, nil
}

func (s *openCodeServer) writeDescriptor(d openCodeDescriptor) error {
	data, err := json.Marshal(d)
	if err != nil {
		return errOpenCodeOwnership
	}
	file, err := os.CreateTemp(s.options.StateDir, ".opencode-server-")
	if err != nil {
		return errOpenCodeOwnership
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return errOpenCodeOwnership
	}
	if err := file.Sync(); err != nil {
		return errOpenCodeOwnership
	}
	if err := file.Close(); err != nil {
		return errOpenCodeOwnership
	}
	if err := os.Rename(file.Name(), s.descriptorPath()); err != nil {
		return errOpenCodeOwnership
	}
	dir, err := os.Open(s.options.StateDir)
	if err != nil {
		return errOpenCodeOwnership
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return errOpenCodeOwnership
	}
	return nil
}

func (s *openCodeServer) alive(d openCodeDescriptor) (bool, error) {
	domain, err := s.domain()
	if err != nil || domain != d.Domain || d.PID == 0 {
		return false, errOpenCodeOwnership
	}
	process, err := s.process(d.PID)
	if err != nil {
		return false, errOpenCodeOwnership
	}
	if process == nil || process.StartTime != d.StartTime {
		return false, nil
	}
	if process.Marker != d.Marker {
		return false, errOpenCodeOwnership
	}
	return true, nil
}

func openCodeOwnsListener(d openCodeDescriptor, process *openCodeProcess) bool {
	u, err := url.Parse(d.URL)
	if err != nil {
		return false
	}
	port, err := strconv.Atoi(u.Port())
	return err == nil && process != nil && process.StartTime == d.StartTime && process.Marker == d.Marker && slices.Contains(process.Ports, port)
}

func (s *openCodeServer) verify(ctx context.Context, d openCodeDescriptor) error {
	ctx, cancel := openCodeTimeout(ctx, s.clock, time.Second)
	defer cancel()
	process, err := s.process(d.PID)
	if err != nil || !openCodeOwnsListener(d, process) {
		return errOpenCodeOwnership
	}
	// Prove socket ownership before transmitting a credential. Check that the
	// server actually requires authentication, not merely that it accepts ours.
	client := &openCodeRPC{endpoint: d.URL, password: "invalid-password", http: s.http, clock: s.clock, ctx: ctx}
	denied, err := client.open(ctx, "GET", "/api/info", nil, "application/json")
	if err != nil {
		return errOpenCodeOwnership
	}
	_ = denied.Body.Close()
	if denied.StatusCode != http.StatusUnauthorized {
		return errOpenCodeOwnership
	}
	client.password = d.Password
	var info struct {
		Version string   `json:"version"`
		PID     int      `json:"pid"`
		URLs    []string `json:"urls"`
	}
	if client.request(ctx, "GET", "/api/info", nil, &info) != nil || info.Version != openCodeVersion || info.PID != d.PID || !slices.Contains(info.URLs, d.URL) {
		return errOpenCodeOwnership
	}
	return nil
}

func (s *openCodeServer) connect(ctx context.Context) (openCodeDescriptor, error) {
	if err := openCodePrivateDirectory(s.options.StateDir, openCodeExistingDirectory); err != nil {
		return openCodeDescriptor{}, errOpenCodeOwnership
	}
	d, err := s.readDescriptor()
	if err != nil {
		return openCodeDescriptor{}, err
	}
	if d == nil || d.PID == 0 {
		return openCodeDescriptor{}, xerrors.New("OpenCode dedicated server is not ready; no service was started")
	}
	alive, err := s.alive(*d)
	if err != nil || !alive {
		return openCodeDescriptor{}, errOpenCodeOwnership
	}
	if err := s.verify(ctx, *d); err != nil {
		return openCodeDescriptor{}, err
	}
	return *d, nil
}

func (s *openCodeServer) lock() (func(), error) {
	path := filepath.Join(s.options.StateDir, "opencode-server.lock")
	// Keep a stable inode. The kernel releases this lock on agent death, so an
	// interrupted startup does not leave an unrecoverable lifecycle lock.
	file, err := openCodePrivateFile(path, os.O_RDWR|os.O_CREATE)
	if err != nil {
		return nil, errOpenCodeOwnership
	}
	if err := openCodeLockFile(file); err != nil {
		_ = file.Close()
		return nil, xerrors.New("OpenCode lifecycle is locked or locking is unavailable")
	}
	unlock := sync.OnceFunc(func() {
		_ = openCodeUnlockFile(file)
		_ = file.Close()
	})
	info, err := file.Stat()
	current, currentErr := os.Lstat(path)
	if err != nil || currentErr != nil || !os.SameFile(info, current) {
		unlock()
		return nil, errOpenCodeOwnership
	}
	return unlock, nil
}

func (s *openCodeServer) pause(ctx context.Context) error {
	timer := s.clock.NewTimer(100*time.Millisecond, "opencode.lifecycle.poll")
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return xerrors.New("OpenCode lifecycle interrupted or timed out")
	case <-timer.C:
		return nil
	}
}

// reusable never interprets a PID in a different boot or PID namespace. A
// foreign-domain record is only replaceable after reserving its unused endpoint.
func (s *openCodeServer) reusable(ctx context.Context, d *openCodeDescriptor) (bool, error) {
	if d == nil {
		return false, nil
	}
	domain, err := s.domain()
	if err != nil {
		return false, errOpenCodeOwnership
	}
	if domain != d.Domain {
		return false, nil
	}
	alive, err := s.alive(*d)
	if err != nil || !alive {
		return false, err
	}
	if err := s.verify(ctx, *d); err != nil {
		return false, err
	}
	return true, nil
}

func (s *openCodeServer) ensure(ctx context.Context) (openCodeDescriptor, error) {
	ctx, cancel := openCodeTimeout(ctx, s.clock, 20*time.Second)
	defer cancel()
	if err := openCodePrivateDirectory(s.options.StateDir, openCodeCreateDirectory); err != nil {
		return openCodeDescriptor{}, errOpenCodeOwnership
	}
	d, err := s.readDescriptor()
	if err != nil {
		return openCodeDescriptor{}, err
	}
	if reusable, err := s.reusable(ctx, d); err != nil {
		return openCodeDescriptor{}, err
	} else if reusable && d.Status == "ready" {
		return *d, nil
	}
	unlock, err := s.lock()
	if err != nil {
		return openCodeDescriptor{}, err
	}
	defer unlock()
	// A second process may have completed startup before this lock was acquired.
	d, err = s.readDescriptor()
	if err != nil {
		return openCodeDescriptor{}, err
	}
	if reusable, err := s.reusable(ctx, d); err != nil {
		return openCodeDescriptor{}, err
	} else if reusable {
		if d.Status == "starting" {
			// Startup may have been canceled after saving the process identity.
			// Authenticated readiness and kernel ownership are checked again
			// under the lifecycle lock before promoting that exact descriptor.
			d.Status = "ready"
			if err := s.writeDescriptor(*d); err != nil {
				return openCodeDescriptor{}, err
			}
		}
		return *d, nil
	}
	if d != nil {
		// Binding, rather than probing with the saved credential, proves there
		// is no reachable listener and excludes port reuse during replacement.
		// This also forces the new server onto a different loopback endpoint.
		u, _ := url.Parse(d.URL)
		retired, err := net.Listen("tcp4", u.Host)
		if err != nil {
			return openCodeDescriptor{}, errOpenCodeOwnership
		}
		defer retired.Close()
	}
	path, err := s.version(ctx)
	if err != nil {
		return openCodeDescriptor{}, err
	}
	domain, err := s.domain()
	if err != nil {
		return openCodeDescriptor{}, errOpenCodeOwnership
	}
	if err := openCodeMakeHome(s.home()); err != nil {
		return openCodeDescriptor{}, err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return openCodeDescriptor{}, xerrors.New("OpenCode private listener unavailable")
	}
	defer listener.Close()
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return openCodeDescriptor{}, xerrors.New("OpenCode private listener is not TCP")
	}
	port := address.Port
	secret := func() string { var data [32]byte; _, _ = rand.Read(data[:]); return hex.EncodeToString(data[:]) }
	d = &openCodeDescriptor{Format: 1, Version: openCodeVersion, URL: "http://127.0.0.1:" + strconv.Itoa(port), Password: secret(), Marker: secret(), Domain: domain, Status: "starting"}
	log, err := openCodePrivateFile(filepath.Join(s.options.StateDir, "opencode-server.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND)
	if err != nil {
		return openCodeDescriptor{}, errOpenCodeOwnership
	}
	defer log.Close()
	if err := s.writeDescriptor(*d); err != nil {
		return openCodeDescriptor{}, err
	}
	if err := ctx.Err(); err != nil {
		return openCodeDescriptor{}, xerrors.New("OpenCode startup interrupted")
	}
	_ = listener.Close()
	// No request context or parent-death signal may terminate this native owner.
	command := s.command(context.Background(), path, "serve", "--hostname", "127.0.0.1", "--port", strconv.Itoa(port))
	command.Dir = s.home()
	command.Env = append(s.environment(), openCodeMarker+"="+d.Marker, "OPENCODE_PASSWORD="+d.Password)
	command.Stdout = log
	command.Stderr = log
	if err := openCodePrepareCommand(command); err != nil {
		return openCodeDescriptor{}, err
	}
	if err := command.Start(); err != nil {
		return openCodeDescriptor{}, xerrors.New("OpenCode dedicated server failed to start; inspect private state")
	}
	exited := make(chan struct{})
	go func() { _ = command.Wait(); close(exited) }()
	for {
		if ctx.Err() != nil {
			return openCodeDescriptor{}, xerrors.New("OpenCode startup timed out; inspect private state")
		}
		select {
		case <-exited:
			return openCodeDescriptor{}, xerrors.New("OpenCode server exited during startup")
		default:
		}
		process, err := s.process(command.Process.Pid)
		if err == nil && process != nil && process.Marker == d.Marker && process.StartTime != "" {
			d.PID = command.Process.Pid
			d.StartTime = process.StartTime
			if err := s.writeDescriptor(*d); err != nil {
				return openCodeDescriptor{}, err
			}
			break
		}
		if err := s.pause(ctx); err != nil {
			return openCodeDescriptor{}, err
		}
	}
	for {
		if ctx.Err() != nil {
			return openCodeDescriptor{}, xerrors.New("OpenCode startup timed out; inspect private state")
		}
		select {
		case <-exited:
			return openCodeDescriptor{}, xerrors.New("OpenCode server exited during startup")
		default:
		}
		if s.verify(ctx, *d) == nil {
			d.Status = "ready"
			if err := s.writeDescriptor(*d); err != nil {
				return openCodeDescriptor{}, err
			}
			return *d, nil
		}
		if err := s.pause(ctx); err != nil {
			return openCodeDescriptor{}, err
		}
	}
}

func (s *openCodeServer) shutdown(ctx context.Context) error {
	ctx, cancel := openCodeTimeout(ctx, s.clock, 20*time.Second)
	defer cancel()
	if _, err := os.Lstat(s.options.StateDir); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err := openCodePrivateDirectory(s.options.StateDir, openCodeExistingDirectory); err != nil {
		return errOpenCodeOwnership
	}
	d, err := s.readDescriptor()
	if err != nil {
		return err
	}
	if d == nil {
		return nil
	}
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	unchanged := func() error {
		next, err := s.readDescriptor()
		if err != nil || next == nil || *next != *d {
			return errOpenCodeOwnership
		}
		return nil
	}
	if err := unchanged(); err != nil {
		return err
	}
	alive, err := s.alive(*d)
	if err != nil || !alive {
		return errOpenCodeOwnership
	}
	if err := s.verify(ctx, *d); err != nil {
		return err
	}
	if err := unchanged(); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return xerrors.New("OpenCode shutdown interrupted")
	}
	if err := s.signal(*d); err != nil {
		return errOpenCodeOwnership
	}
	for {
		alive, err := s.alive(*d)
		if err == nil && !alive {
			break
		}
		// Teardown can remove /proc environment and FD entries before stat
		// reports exit. Keep the descriptor until death is positively proved.
		// Unverifiable ownership times out without any additional signal.
		if err := s.pause(ctx); err != nil {
			return err
		}
	}
	if err := unchanged(); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return xerrors.New("OpenCode shutdown interrupted")
	}
	if err := os.Remove(s.descriptorPath()); err != nil {
		return errOpenCodeOwnership
	}
	return nil
}
