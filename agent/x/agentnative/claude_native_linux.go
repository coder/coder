//go:build linux

package agentnative

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"
)

const claudeResumeRefused = "cannot safely resume native Claude; inspect its saved state and transcript in the native terminal; the original task was not rerun"

// Permission masks reject access by other users, or only writes when native
// state is intentionally readable outside the owning user.
const (
	claudePrivateMask  os.FileMode = 0o077
	claudeReadOnlyMask os.FileMode = 0o022
)

type claudeRegistryMode uint8

const (
	claudeRegistryComplete claudeRegistryMode = iota
	claudeRegistryPartial
)

// Native schemas are deliberately version-specific. Unknown fields are retained
// for queue-presence checks; unknown process domains and ownership fail closed.
type claudeNativeState struct {
	SessionID       string          `json:"sessionId"`
	Cwd             string          `json:"cwd"`
	State           string          `json:"state"`
	Tempo           string          `json:"tempo"`
	Backend         string          `json:"backend"`
	CLIVersion      string          `json:"cliVersion"`
	ResumeSessionID string          `json:"resumeSessionId"`
	DaemonShort     string          `json:"daemonShort"`
	SessionIDTaken  bool            `json:"sessionIdTaken"`
	Template        string          `json:"template"`
	Routine         json.RawMessage `json:"routine"`
	RespawnFlags    []string        `json:"respawnFlags"`
	LinkScanPath    string          `json:"linkScanPath"`
	InFlight        struct {
		Queued int `json:"queued"`
	} `json:"inFlight"`
}

type claudeNativeAgent struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	SessionID string          `json:"sessionId"`
	Cwd       string          `json:"cwd"`
	State     string          `json:"state"`
	PID       json.RawMessage `json:"pid"`
	Status    json.RawMessage `json:"status"`
}

type claudeNativeLock struct {
	PID     int    `json:"pid"`
	Version string `json:"version"`
}
type claudeNativeWorker struct {
	PID        int    `json:"pid"`
	ProcStart  string `json:"procStart,omitempty"`
	ReplPID    *int   `json:"replPid"`
	SessionID  string `json:"sessionId"`
	Cwd        string `json:"cwd"`
	CLIVersion string `json:"cliVersion"`
}
type claudeNativeRoster struct {
	Proto         int                           `json:"proto"`
	SupervisorPID int                           `json:"supervisorPid"`
	Workers       map[string]claudeNativeWorker `json:"workers"`
}
type claudeNativeSession struct {
	PID       int    `json:"pid"`
	ProcStart string `json:"procStart,omitempty"`
	Version   string `json:"version"`
	PIDDomain string `json:"pidDomain"`
	SessionID string `json:"sessionId"`
	Kind      string `json:"kind"`
	JobID     string `json:"jobId"`
	Cwd       string `json:"cwd"`
}

type claudeFileSnapshot struct {
	path string
	data []byte
	mask os.FileMode
}
type claudeProcessSnapshot struct {
	files    []claudeFileSnapshot
	names    []string
	pids     map[int]bool
	workers  []claudeNativeWorker
	sessions []claudeNativeSession
}

func claudeStat(info os.FileInfo) (*syscall.Stat_t, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, xerrors.New("cannot verify native file ownership")
	}
	return stat, nil
}

func claudeAncestors(path string) error {
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		stat, err := claudeStat(info)
		if err != nil {
			return err
		}
		if !info.IsDir() || (int64(stat.Uid) != int64(os.Geteuid()) && stat.Uid != 0) || (info.Mode().Perm()&0o022 != 0 && info.Mode()&os.ModeSticky == 0) {
			return xerrors.New("unsafe native path ancestor")
		}
		if filepath.Dir(current) == current {
			return nil
		}
	}
}

func claudeDirectory(path string, mask os.FileMode) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, err := claudeStat(info)
	if err != nil {
		return err
	}
	if !info.IsDir() || int64(stat.Uid) != int64(os.Geteuid()) || info.Mode().Perm()&mask != 0 {
		return xerrors.New("unsafe native directory")
	}
	return nil
}

func claudeMakePrivate(path string) error {
	if !nativeValidPath(path) {
		return xerrors.New("invalid private path")
	}
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		parent := filepath.Dir(path)
		if _, err := os.Lstat(parent); errors.Is(err, os.ErrNotExist) {
			if err := claudeMakePrivate(parent); err != nil {
				return err
			}
		}
		if err := claudeAncestors(parent); err != nil {
			return err
		}
		if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
	}
	if err := claudeAncestors(path); err != nil {
		return err
	}
	return claudeDirectory(path, claudePrivateMask)
}

func claudeOpenPrivate(path string, mask os.FileMode) (*os.File, os.FileInfo, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err == nil {
		var stat *syscall.Stat_t
		stat, err = claudeStat(info)
		if err == nil && (!info.Mode().IsRegular() || int64(stat.Uid) != int64(os.Geteuid()) || stat.Nlink != 1 || info.Mode().Perm()&mask != 0) {
			err = xerrors.New("unsafe native state file")
		}
	}
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	return file, info, nil
}

func claudeReadPrivate(path string, mask os.FileMode, limit int64) ([]byte, error) {
	file, info, err := claudeOpenPrivate(path, mask)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if info.Size() > limit {
		return nil, xerrors.New("native state exceeds limit")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, xerrors.New("native state exceeds limit")
	}
	return data, nil
}

func claudeWritePrivate(path string, data []byte) error {
	if err := claudeDirectory(filepath.Dir(path), claudePrivateMask); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		file, _, err := claudeOpenPrivate(path, claudePrivateMask)
		if err != nil {
			return err
		}
		_ = file.Close()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary := filepath.Join(filepath.Dir(path), ".claude-"+uuid.NewString())
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer os.Remove(temporary)
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func (c *Claude) nativeState(session Session, launchID string) (claudeNativeState, []byte, error) {
	var state claudeNativeState
	jobs := filepath.Join(c.root, "jobs")
	job := filepath.Join(jobs, session.NativeID[:8])
	if err := claudeAncestors(c.root); err != nil {
		return state, nil, err
	}
	for _, path := range []string{c.root, jobs, job} {
		mask := claudeReadOnlyMask
		if path == c.root {
			mask = claudePrivateMask
		}
		if err := claudeDirectory(path, mask); err != nil {
			return state, nil, err
		}
	}
	data, err := claudeReadPrivate(filepath.Join(job, "state.json"), claudePrivateMask, 256*1024)
	if err != nil {
		return state, nil, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, nil, err
	}
	if state.SessionID != session.NativeID || state.Cwd != session.Cwd || state.State == "" || (state.ResumeSessionID != "" && state.ResumeSessionID != session.NativeID) || (state.DaemonShort != "" && state.DaemonShort != session.NativeID[:8]) || state.SessionIDTaken {
		return state, nil, xerrors.New("native identity mismatch")
	}
	if !c.ownsFlags(state.RespawnFlags, launchID) {
		return state, nil, xerrors.New("native ownership mismatch")
	}
	return state, data, nil
}

func (c *Claude) ownsFlags(flags []string, launchID string) bool {
	if len(flags) > 256 {
		return false
	}
	var settings, plugins []string
	for i, flag := range flags {
		if strings.HasPrefix(flag, "--settings=") || strings.HasPrefix(flag, "--plugin-dir=") {
			return false
		}
		if flag == "--settings" || flag == "--plugin-dir" {
			if i+1 >= len(flags) {
				return false
			}
			if flag == "--settings" {
				settings = append(settings, flags[i+1])
			} else {
				plugins = append(plugins, flags[i+1])
			}
		}
	}
	if len(settings) != 1 || len(plugins) != 1 || plugins[0] != c.plugin {
		return false
	}
	var config struct {
		Env map[string]string `json:"env"`
	}
	if json.Unmarshal([]byte(settings[0]), &config) != nil {
		return false
	}
	return config.Env["WORKSPACE_BRIDGE_FILE"] == c.descriptor && config.Env["WORKSPACE_BRIDGE_LAUNCH_ID"] == launchID && claudeUUID(launchID)
}

func claudeEnded(state string) bool {
	return slices.Contains([]string{"ended", "stopped", "exited", "completed", "done", "failed"}, state)
}

func (c *Claude) verifyResume(ctx context.Context, session Session, launchID string) error {
	if err := c.validateResume(ctx, session, launchID); err != nil {
		if errors.Is(err, errClaudeLiveWorker) {
			return errClaudeLiveWorker
		}
		return xerrors.Errorf("%s: %w", claudeResumeRefused, ErrUnsupported)
	}
	return nil
}

func (c *Claude) validateResume(ctx context.Context, session Session, launchID string) error {
	state, data, err := c.nativeState(session, launchID)
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	output, err := c.run(ctx, session.Cwd, 5*time.Second, "agents", "--json", "--all")
	if err != nil {
		return err
	}
	var agents []claudeNativeAgent
	if err := json.Unmarshal(output, &agents); err != nil || agents == nil || len(agents) > 256 {
		return xerrors.New("invalid native agent list")
	}
	var matches []claudeNativeAgent
	for _, agent := range agents {
		if agent.SessionID == session.NativeID {
			matches = append(matches, agent)
		}
	}
	if len(matches) != 1 {
		return xerrors.New("native agent identity is ambiguous")
	}
	agent := matches[0]
	if agent.Kind != "background" || agent.ID != session.NativeID[:8] || agent.Cwd != session.Cwd {
		return xerrors.New("native agent ownership mismatch")
	}
	// A positive pid is live even if a completed turn marks the job ended.
	if len(agent.PID) != 0 {
		var pid int
		if json.Unmarshal(agent.PID, &pid) == nil && pid > 1 && pid <= 2147483647 {
			return errClaudeLiveWorker
		}
		return xerrors.New("native worker may still be running")
	}
	if _, queued := fields["queuedPrompt"]; queued || state.InFlight.Queued != 0 {
		return xerrors.New("native session has queued work")
	}
	if state.Template != "bg" || (len(state.Routine) != 0 && string(state.Routine) != "null" && string(state.Routine) != "false") {
		return xerrors.New("unsupported native template")
	}
	for _, flag := range state.RespawnFlags {
		for _, forbidden := range []string{"--reply-on-resume", "--agent", "--routine"} {
			if flag == forbidden || strings.HasPrefix(flag, forbidden+"=") {
				return xerrors.New("unsafe native resume flag")
			}
		}
	}
	var stopped *claudeProcessSnapshot
	if !claudeEnded(state.State) && !claudeEnded(agent.State) {
		if agent.State != "blocked" || len(agent.Status) != 0 || state.State != "blocked" || state.Tempo != "blocked" || state.CLIVersion != "2.1.309" || state.Backend != "daemon" {
			return xerrors.New("native worker is not stopped")
		}
		stopped, err = c.stoppedSnapshot(session)
		if err != nil {
			return err
		}
	}
	if err := c.verifyTranscript(state, session); err != nil {
		return err
	}
	if err := c.verifyResumeProcesses(session.NativeID, stopped); err != nil {
		return err
	}
	current, err := claudeReadPrivate(filepath.Join(c.root, "jobs", session.NativeID[:8], "state.json"), claudePrivateMask, 256*1024)
	if err != nil || !bytes.Equal(current, data) {
		return xerrors.New("native state changed during validation")
	}
	return ctx.Err()
}

// verifyResumeProcesses also checks ended jobs for stale registry workers. A
// live process must never be fed to --bg --resume, which could silently clone it.
func (c *Claude) verifyResumeProcesses(id string, stopped *claudeProcessSnapshot) error {
	if stopped != nil {
		for pid := range stopped.pids {
			if err := claudeRequireDead(pid); err != nil {
				return err
			}
		}
		return c.recheckSnapshot(stopped)
	}
	snapshot, err := c.processSnapshot(claudeRegistryPartial)
	if err != nil {
		return err
	}
	for _, worker := range snapshot.workers {
		if worker.SessionID != id {
			continue
		}
		if err := claudeRequireDead(worker.PID); err != nil {
			return err
		}
		if worker.ReplPID != nil {
			if err := claudeRequireDead(*worker.ReplPID); err != nil {
				return err
			}
		}
	}
	for _, record := range snapshot.sessions {
		if record.SessionID == id {
			if err := claudeRequireDead(record.PID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Claude) verifyTranscript(state claudeNativeState, session Session) error {
	projects := filepath.Join(c.root, "projects")
	path := state.LinkScanPath
	if path == "" {
		project := regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(session.Cwd, "-")
		if len(project) > 200 {
			return xerrors.New("native transcript path is ambiguous")
		}
		path = filepath.Join(projects, project, session.NativeID+".jsonl")
	}
	if !nativeValidPath(path) || filepath.Base(path) != session.NativeID+".jsonl" || filepath.Dir(filepath.Dir(path)) != projects {
		return xerrors.New("invalid native transcript path")
	}
	for _, directory := range []string{projects, filepath.Dir(path)} {
		if err := claudeDirectory(directory, claudeReadOnlyMask); err != nil {
			return err
		}
	}
	file, info, err := claudeOpenPrivate(path, claudePrivateMask)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 256*1024))
	if err != nil || len(data) == 0 {
		return xerrors.New("native transcript is empty")
	}
	lines := bytes.Split(data, []byte{'\n'})
	if int64(len(data)) < info.Size() {
		lines = lines[:len(lines)-1]
	}
	for _, line := range lines {
		if !bytes.Contains(line, []byte(`"type":"user"`)) && !bytes.Contains(line, []byte(`"type":"assistant"`)) {
			continue
		}
		var message struct {
			SessionID string                     `json:"sessionId"`
			Type      string                     `json:"type"`
			Message   map[string]json.RawMessage `json:"message"`
		}
		if json.Unmarshal(line, &message) == nil && message.SessionID == session.NativeID && (message.Type == "user" || message.Type == "assistant") && message.Message != nil {
			return nil
		}
	}
	return xerrors.New("native transcript has no resumable messages")
}

func claudePIDDomain() (string, error) {
	var machine string
	fd, err := unix.Open("/etc/machine-id", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil && !errors.Is(err, unix.ENOENT) {
		return "", err
	}
	if err == nil {
		file := os.NewFile(uintptr(fd), "/etc/machine-id")
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() > 64 {
			return "", xerrors.New("invalid machine identity file")
		}
		data, err := io.ReadAll(io.LimitReader(file, 65))
		if err != nil || len(data) > 64 {
			return "", xerrors.New("invalid machine identity file")
		}
		machine = strings.TrimSpace(string(data))
	}
	if machine != "" && !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(machine) {
		return "", xerrors.New("invalid native process domain")
	}
	namespace, err := os.Readlink("/proc/self/ns/pid")
	if err != nil {
		return "", err
	}
	return "linux:" + machine + ":" + namespace, nil
}

func claudeRequireDead(pid int) error {
	if pid <= 1 || pid > 2147483647 {
		return xerrors.New("invalid native pid")
	}
	if err := unix.Kill(pid, 0); !errors.Is(err, unix.ESRCH) {
		return xerrors.New("native process is live or ambiguous")
	}
	return nil
}

func claudeRegistryNames(path string, mode claudeRegistryMode) ([]string, error) {
	file, err := os.Open(path)
	if mode == claudeRegistryPartial && errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	entries, err := file.ReadDir(257)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > 256 {
		return nil, xerrors.New("native registry exceeds limit")
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	slices.Sort(names)
	return names, nil
}

func (c *Claude) processSnapshot(mode claudeRegistryMode) (*claudeProcessSnapshot, error) {
	if err := claudeAncestors(c.root); err != nil {
		return nil, err
	}
	if err := claudeDirectory(c.root, claudePrivateMask); err != nil {
		return nil, err
	}
	for _, dir := range []string{filepath.Join(c.root, "daemon"), filepath.Join(c.root, "sessions")} {
		if err := claudeDirectory(dir, claudePrivateMask); err != nil && (mode != claudeRegistryPartial || !errors.Is(err, os.ErrNotExist)) {
			return nil, err
		}
	}
	snapshot := &claudeProcessSnapshot{pids: make(map[int]bool)}
	total := 0
	read := func(path string, mask os.FileMode, target any) (bool, error) {
		data, err := claudeReadPrivate(path, mask, 256*1024)
		if mode == claudeRegistryPartial && errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		total += len(data)
		if total > 1024*1024 {
			return false, xerrors.New("native registry exceeds limit")
		}
		if err := json.Unmarshal(data, target); err != nil {
			return false, err
		}
		snapshot.files = append(snapshot.files, claudeFileSnapshot{path: path, mask: mask, data: data})
		return true, nil
	}
	capture := func(pid int) error {
		if pid <= 1 || pid > 2147483647 {
			return xerrors.New("invalid native registry pid")
		}
		snapshot.pids[pid] = true
		return nil
	}
	var lock claudeNativeLock
	hasLock, err := read(filepath.Join(c.root, "daemon.lock"), claudeReadOnlyMask, &lock)
	if err != nil {
		return nil, err
	}
	if hasLock {
		if lock.Version != "2.1.309" {
			return nil, xerrors.New("unsupported native daemon version")
		}
		if err := capture(lock.PID); err != nil {
			return nil, err
		}
	}
	var roster claudeNativeRoster
	hasRoster, err := read(filepath.Join(c.root, "daemon", "roster.json"), claudePrivateMask, &roster)
	if err != nil {
		return nil, err
	}
	if hasRoster {
		if roster.Proto != 1 || roster.Workers == nil || len(roster.Workers) > 256 || (hasLock && roster.SupervisorPID != lock.PID) {
			return nil, xerrors.New("ambiguous native daemon roster")
		}
		if err := capture(roster.SupervisorPID); err != nil {
			return nil, err
		}
		for short, worker := range roster.Workers {
			if !claudeUUID(worker.SessionID) || short != worker.SessionID[:8] || worker.CLIVersion != "2.1.309" || !nativeValidPath(worker.Cwd) {
				return nil, xerrors.New("ambiguous native worker")
			}
			if err := capture(worker.PID); err != nil {
				return nil, err
			}
			if worker.ReplPID != nil {
				if err := capture(*worker.ReplPID); err != nil {
					return nil, err
				}
			}
			snapshot.workers = append(snapshot.workers, worker)
		}
	}
	snapshot.names, err = claudeRegistryNames(filepath.Join(c.root, "sessions"), mode)
	if err != nil {
		return nil, err
	}
	domain, err := claudePIDDomain()
	if err != nil {
		return nil, err
	}
	for _, name := range snapshot.names {
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		var record claudeNativeSession
		present, err := read(filepath.Join(c.root, "sessions", name), claudeReadOnlyMask, &record)
		if err != nil {
			return nil, err
		}
		if !present {
			return nil, xerrors.New("native registry changed during scan")
		}
		if record.Version != "2.1.309" || record.PIDDomain != domain || name != strconv.Itoa(record.PID)+".json" || !claudeUUID(record.SessionID) {
			return nil, xerrors.New("ambiguous native session registry")
		}
		if err := capture(record.PID); err != nil {
			return nil, err
		}
		snapshot.sessions = append(snapshot.sessions, record)
	}
	return snapshot, nil
}

func (c *Claude) recheckSnapshot(snapshot *claudeProcessSnapshot) error {
	for i := len(snapshot.files) - 1; i >= 0; i-- {
		file := snapshot.files[i]
		data, err := claudeReadPrivate(file.path, file.mask, 256*1024)
		if err != nil || !bytes.Equal(data, file.data) {
			return xerrors.New("native registry changed during validation")
		}
	}
	names, err := claudeRegistryNames(filepath.Join(c.root, "sessions"), claudeRegistryPartial)
	if err != nil || !slices.Equal(names, snapshot.names) {
		return xerrors.New("native registry membership changed")
	}
	return nil
}

func (c *Claude) stoppedSnapshot(session Session) (*claudeProcessSnapshot, error) {
	snapshot, err := c.processSnapshot(claudeRegistryComplete)
	if err != nil {
		return nil, err
	}
	workers, records := 0, 0
	for _, worker := range snapshot.workers {
		if worker.SessionID == session.NativeID {
			if worker.Cwd != session.Cwd {
				return nil, xerrors.New("native worker directory mismatch")
			}
			workers++
		}
	}
	for _, record := range snapshot.sessions {
		if record.SessionID == session.NativeID {
			if record.Kind != "bg" || record.JobID != session.NativeID[:8] || record.Cwd != session.Cwd {
				return nil, xerrors.New("native registry ownership mismatch")
			}
			records++
		}
	}
	if workers != 1 || records != 1 {
		return nil, xerrors.New("native registry identity is ambiguous")
	}
	for pid := range snapshot.pids {
		if err := claudeRequireDead(pid); err != nil {
			return nil, err
		}
	}
	if err := c.recheckSnapshot(snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

// Shutdown stops individually verified adapter-owned workers. The shared native
// supervisor is never stopped: its roster does not describe its entire PTY domain.
func (c *Claude) Shutdown(ctx context.Context) error {
	if err := c.stopOwnedWorkers(ctx); err != nil {
		return xerrors.Errorf("native Claude shutdown could not be verified; saved state was not repaired: %w", ErrUnsupported)
	}
	return nil
}

func (c *Claude) stopOwnedWorkers(ctx context.Context) error {
	if _, err := os.Lstat(c.root); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	snapshot, err := c.processSnapshot(claudeRegistryPartial)
	if err != nil {
		return err
	}
	for _, target := range snapshot.workers {
		session, err := c.options.Host.Session(target.SessionID)
		if err != nil || session.Harness != "claude" {
			// Native workers not registered with this adapter are not ours.
			continue
		}
		if session.ID != target.SessionID || session.NativeID != target.SessionID || session.Cwd != target.Cwd {
			return xerrors.New("native shutdown identity mismatch")
		}
		var state claudeState
		if json.Unmarshal(session.State, &state) != nil || !claudeUUID(state.LaunchID) {
			return xerrors.New("unverified native session")
		}
		_, data, err := c.nativeState(session, state.LaunchID)
		if err != nil {
			return err
		}
		// Stopping an earlier worker changes the roster. Revalidate each target
		// independently instead of treating an old roster as a kill authority.
		current, err := c.processSnapshot(claudeRegistryPartial)
		if err != nil {
			return err
		}
		pids, err := claudeShutdownPIDs(current, target)
		if err != nil {
			return err
		}
		if err := c.recheckSnapshot(current); err != nil {
			return err
		}
		_, rechecked, err := c.nativeState(session, state.LaunchID)
		if err != nil || !bytes.Equal(data, rechecked) {
			return xerrors.New("native state changed during shutdown validation")
		}
		// Native stop addresses one verified job, never the supervisor or its
		// unlisted PTYs. Do not signal saved PIDs or process groups ourselves.
		if _, err := c.run(ctx, session.Cwd, 10*time.Second, "stop", session.NativeID[:8]); err != nil {
			return err
		}
		if err := c.waitWorkerStopped(ctx, session.NativeID, pids); err != nil {
			return err
		}
	}
	return nil
}

// claudeShutdownPIDs binds a job's native ownership to current process start
// identities. A reused PID or a missing live-process identity fails closed.
func claudeShutdownPIDs(snapshot *claudeProcessSnapshot, target claudeNativeWorker) (map[int]bool, error) {
	var worker *claudeNativeWorker
	for i := range snapshot.workers {
		candidate := &snapshot.workers[i]
		if candidate.SessionID == target.SessionID {
			worker = candidate
		}
	}
	if worker == nil || worker.PID != target.PID || worker.ProcStart != target.ProcStart || worker.Cwd != target.Cwd || (worker.ReplPID == nil) != (target.ReplPID == nil) || (worker.ReplPID != nil && *worker.ReplPID != *target.ReplPID) {
		return nil, xerrors.New("native worker changed during shutdown validation")
	}
	if err := claudeVerifyProcess(worker.PID, worker.ProcStart); err != nil {
		return nil, err
	}
	pids := map[int]bool{worker.PID: true}
	replVerified := worker.ReplPID == nil
	records := 0
	for _, record := range snapshot.sessions {
		if record.SessionID != worker.SessionID {
			continue
		}
		if record.Kind != "bg" || record.JobID != worker.SessionID[:8] || record.Cwd != worker.Cwd || (record.PID != worker.PID && (worker.ReplPID == nil || record.PID != *worker.ReplPID)) {
			return nil, xerrors.New("native worker registry mismatch")
		}
		if err := claudeVerifyProcess(record.PID, record.ProcStart); err != nil {
			return nil, err
		}
		pids[record.PID] = true
		records++
		if worker.ReplPID != nil && record.PID == *worker.ReplPID {
			replVerified = true
		}
	}
	if records == 0 || !replVerified {
		return nil, xerrors.New("native worker process identity is incomplete")
	}
	return pids, nil
}

func claudeVerifyProcess(pid int, expected string) error {
	if err := claudeRequireDead(pid); err == nil {
		return nil
	}
	if expected == "" {
		return xerrors.New("native process start identity is missing")
	}
	actual, err := claudeProcessStart(pid)
	if err != nil || actual != expected {
		return xerrors.New("native process start identity changed")
	}
	return nil
}

func claudeProcessStart(pid int) (string, error) {
	if pid <= 1 || pid > 2147483647 {
		return "", xerrors.New("invalid native pid")
	}
	file, err := os.Open(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	stat, err := claudeStat(info)
	if err != nil || int64(stat.Uid) != int64(os.Geteuid()) {
		return "", xerrors.New("native process owner mismatch")
	}
	data, err := io.ReadAll(io.LimitReader(file, 8193))
	if err != nil || len(data) > 8192 {
		return "", xerrors.New("cannot read native process identity")
	}
	// The command name can contain spaces and parentheses. Start time is field
	// 22, or field 20 after the final command-name delimiter.
	end := bytes.LastIndexByte(data, ')')
	if end < 0 {
		return "", xerrors.New("invalid native process stat")
	}
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) < 20 {
		return "", xerrors.New("invalid native process stat")
	}
	if _, err := strconv.ParseUint(fields[19], 10, 64); err != nil {
		return "", err
	}
	return fields[19], nil
}

func (c *Claude) waitWorkerStopped(ctx context.Context, id string, pids map[int]bool) error {
	timer := c.clock.NewTimer(45*time.Second, "claude", "shutdown")
	defer timer.Stop()
	ticker := c.clock.NewTicker(200*time.Millisecond, "claude", "shutdown_poll")
	defer ticker.Stop()
	for {
		current, err := c.processSnapshot(claudeRegistryPartial)
		if err != nil {
			return err
		}
		clean := true
		for _, worker := range current.workers {
			if worker.SessionID == id {
				clean = false
			}
		}
		for _, record := range current.sessions {
			if record.SessionID == id {
				clean = false
			}
		}
		for pid := range pids {
			if claudeRequireDead(pid) != nil {
				clean = false
			}
		}
		if clean {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return context.DeadlineExceeded
		case <-ticker.C:
		}
	}
}
