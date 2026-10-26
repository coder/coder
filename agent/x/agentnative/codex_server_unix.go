//go:build linux

package agentnative

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"
)

const codexProcessMarker = "CODER_CODEX_SERVER_INSTANCE"

//nolint:revive // create must remain false on attachment and shutdown paths.
func (c *Codex) privateDirectory(create bool) error {
	if create {
		if err := os.MkdirAll(c.options.StateDir, 0o700); err != nil {
			return xerrors.New("create private codex state directory")
		}
	}
	info, err := os.Lstat(c.options.StateDir)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode().Perm()&0o077 != 0 || int64(stat.Uid) != int64(os.Geteuid()) {
		return errCodexOwnership
	}
	return nil
}

func codexPrivateFile(path string, flags int) (*os.File, error) {
	f, err := os.OpenFile(path, flags|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0o600)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err == nil {
		stat, ok := info.Sys().(*syscall.Stat_t)
		if ok && info.Mode().IsRegular() && info.Mode().Perm()&0o077 == 0 && int64(stat.Uid) == int64(os.Geteuid()) && stat.Nlink == 1 {
			return f, nil
		}
	}
	_ = f.Close()
	return nil, errCodexOwnership
}

func (c *Codex) serverRecordPath() string {
	return filepath.Join(c.options.StateDir, "codex-server.json")
}

//nolint:nilnil // A missing optional ownership record is distinct from a corrupt one.
func (c *Codex) readServerRecord() (*codexServerRecord, error) {
	f, err := codexPrivateFile(c.serverRecordPath(), os.O_RDONLY)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errCodexOwnership
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 8193))
	var record codexServerRecord
	if err != nil || len(data) > 8192 || json.Unmarshal(data, &record) != nil || record.Version != codexVersion || !codexValidID(record.Marker) || record.Boot == "" || record.Namespace == "" || record.Socket != filepath.Join(c.options.StateDir, "codex.sock") || record.Leader.PID <= 1 || record.Leader.Group != record.Leader.PID || record.Leader.Session != record.Leader.PID || record.Leader.Start == "" || record.Leader.Executable == "" || record.Leader.Inode == 0 {
		return nil, errCodexOwnership
	}
	return &record, nil
}

func (c *Codex) writeServerRecord(record codexServerRecord) error {
	temporary := filepath.Join(c.options.StateDir, ".codex-server-"+uuid.NewString()+".tmp")
	f, err := codexPrivateFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return err
	}
	defer f.Close()
	defer os.Remove(temporary)
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := os.Rename(temporary, c.serverRecordPath()); err != nil {
		return err
	}
	dir, err := os.Open(c.options.StateDir)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// checkServerStart retires a foreign container's local ownership record only
// after proving the socket is closed. A process cannot move PID namespaces, so
// the old numeric PID is not an identity in this namespace and is never signaled.
// Native history is kept; only a new launch replaces the obsolete owner record.
func (c *Codex) checkServerStart(ctx context.Context) error {
	if _, err := c.socketPeer(ctx); !errors.Is(err, errCodexSocketClosed) {
		return errCodexOwnership
	}
	record, err := c.readServerRecord()
	if err != nil || record == nil {
		return err
	}
	boot, namespace, err := codexProcessDomain()
	if err != nil {
		return err
	}
	if record.Boot != boot {
		return nil
	}
	if record.Namespace != namespace {
		return nil
	}
	members, err := codexOwnedProcesses(*record)
	defer codexCloseProcesses(members)
	if err != nil || len(members) > 0 {
		return errCodexOwnership
	}
	return nil
}

// prepareHome initializes only the native directory. Authentication and
// configuration remain under the workspace user's control.
func (c *Codex) prepareHome() error {
	var home, codexHome string
	for _, entry := range c.options.Environment {
		key, value, _ := strings.Cut(entry, "=")
		switch key {
		case "HOME":
			home = value
		case "CODEX_HOME":
			codexHome = value
		}
	}
	if codexHome == "" {
		if !filepath.IsAbs(home) {
			return xerrors.New("native Codex requires an absolute HOME or CODEX_HOME")
		}
		codexHome = filepath.Join(home, ".codex")
	}
	if !filepath.IsAbs(codexHome) {
		return xerrors.New("native Codex requires an absolute CODEX_HOME")
	}
	dir, err := openPrivateDirectory(codexHome, privateDirectoryCreate)
	if err != nil {
		return xerrors.Errorf("prepare native Codex home: %w", err)
	}
	return dir.close()
}

func (c *Codex) startServer() error {
	if err := c.prepareHome(); err != nil {
		return err
	}
	log, err := codexPrivateFile(filepath.Join(c.options.StateDir, "codex-server.log"), os.O_WRONLY|os.O_APPEND|os.O_CREATE)
	if err != nil {
		return err
	}
	defer log.Close()
	boot, namespace, err := codexProcessDomain()
	if err != nil {
		return err
	}
	cmd, err := c.command(context.Background(), "app-server", "--listen", "unix://"+filepath.Join(c.options.StateDir, "codex.sock"))
	if err != nil {
		return err
	}
	marker := uuid.NewString()
	for _, entry := range c.options.Environment {
		if !strings.HasPrefix(entry, codexProcessMarker+"=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, codexProcessMarker+"="+marker)
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return xerrors.New("start native codex app-server; inspect the private log")
	}
	done := make(chan struct{})
	c.owned = cmd
	c.ownedDone = done
	c.ownedFinalized = false
	go func() { _ = cmd.Wait(); close(done) }()
	identity, err := c.startedProcess(cmd.Process.Pid, marker)
	if err != nil {
		// The Go process handle identifies exactly this child, not a reused PID.
		_ = cmd.Process.Kill()
		return errCodexOwnership
	}
	record := codexServerRecord{Version: codexVersion, Boot: boot, Namespace: namespace, Marker: marker, Socket: filepath.Join(c.options.StateDir, "codex.sock"), Leader: *identity}
	c.ownedRecord = &record
	if err := c.writeServerRecord(record); err != nil {
		_ = c.killOwnedGroup()
		return err
	}
	return nil
}

// Start returns once exec closes its error pipe, which can precede publication
// of the new /proc image. Wait for the inherited marker instead of persisting a
// transient parent executable or environment as the native owner's identity.
func (c *Codex) startedProcess(pid int, marker string) (*codexProcessRecord, error) {
	ctx, cancel := codexTimeout(context.Background(), c.clock, time.Second, "codex-process-start")
	defer cancel()
	for {
		identity, currentMarker, err := codexProcessIdentity(pid)
		if err == nil && identity != nil && currentMarker == marker && identity.Group == pid && identity.Session == pid {
			return identity, nil
		}
		timer := c.clock.NewTimer(time.Millisecond, "codex-process-poll")
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, errCodexOwnership
		case <-c.ownedDone:
			timer.Stop()
			return nil, errCodexOwnership
		case <-timer.C:
		}
	}
}

// finalizeServerIdentity records the post-exec image once the native service is
// ready. Only this instance's unreaped child can refresh the executable identity;
// a reopened observer must verify the persisted image without changing it.
func (c *Codex) finalizeServerIdentity() error {
	if c.owned == nil || c.ownedRecord == nil || c.ownedFinalized {
		return nil
	}
	record, err := c.readServerRecord()
	if err != nil || record == nil || *record != *c.ownedRecord {
		return errCodexOwnership
	}
	identity, marker, err := codexProcessIdentity(record.Leader.PID)
	if err != nil {
		return err
	}
	if identity == nil {
		members, err := codexOwnedProcesses(*record)
		defer codexCloseProcesses(members)
		if err != nil || len(members) == 0 {
			return errCodexOwnership
		}
		c.ownedFinalized = true
		return nil
	}
	if marker != record.Marker || identity.Start != record.Leader.Start || identity.Group != record.Leader.Group || identity.Session != record.Leader.Session {
		return errCodexOwnership
	}
	record.Leader = *identity
	if err := c.writeServerRecord(*record); err != nil {
		return err
	}
	c.ownedRecord = record
	c.ownedFinalized = true
	return nil
}

func (c *Codex) killOwnedGroup() error {
	if c.ownedRecord == nil {
		return errCodexOwnership
	}
	members, err := codexOwnedProcesses(*c.ownedRecord)
	defer codexCloseProcesses(members)
	if err != nil {
		return err
	}
	return codexSignalProcesses(members, unix.SIGKILL)
}

// stopOwned loads the durable identity even when this observer never started
// the server. Marked members are verified and pinned with a pidfd before
// signaling; unmarked tools are left alone. Wrapper exit does not imply native
// child exit. No numeric PID or
// process-group signal is used, since group IDs can be recycled during drain.
func (c *Codex) stopOwned(ctx context.Context) error {
	if err := c.privateDirectory(false); errors.Is(err, os.ErrNotExist) {
		c.mu.Lock()
		seen := c.seenServer
		c.mu.Unlock()
		if seen || c.ownedRecord != nil || c.owned != nil {
			return errCodexOwnership
		}
		return nil
	} else if err != nil {
		return err
	}
	record, err := c.readServerRecord()
	if err != nil {
		return err
	}
	if record == nil {
		c.mu.Lock()
		seen := c.seenServer
		c.mu.Unlock()
		_, socketErr := os.Lstat(filepath.Join(c.options.StateDir, "codex.sock"))
		if seen || c.owned != nil || !errors.Is(socketErr, os.ErrNotExist) {
			return errCodexOwnership
		}
		return nil
	}
	boot, namespace, err := codexProcessDomain()
	if err != nil || namespace != record.Namespace {
		return errCodexOwnership
	}
	if boot != record.Boot {
		return errCodexOwnership
	}
	peer, err := c.socketPeer(ctx)
	if !errors.Is(err, errCodexSocketClosed) && (err != nil || !codexSocketMatches(*record, peer)) {
		return errCodexOwnership
	}
	start := c.clock.Now()
	terminating := make(map[codexProcessRecord]bool)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		current, err := c.readServerRecord()
		if err != nil || current == nil || *current != *record {
			return errCodexOwnership
		}
		members, err := codexOwnedProcesses(*record)
		if err != nil {
			codexCloseProcesses(members)
			// Exit teardown can remove environ/exe before the process becomes a
			// zombie. After a verified signal, wait for proof of death without
			// issuing any more signals based on incomplete process identity.
			if len(terminating) == 0 || c.clock.Since(start) >= 10*time.Second {
				return err
			}
			timer := c.clock.NewTimer(10*time.Millisecond, "codex-shutdown-retry")
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if len(members) == 0 {
			// A dead owner is not proof that the rendezvous still belongs to it.
			// Refuse success if another listener replaced the native service.
			if _, err := c.socketPeer(ctx); !errors.Is(err, errCodexSocketClosed) {
				return errCodexOwnership
			}
			c.owned = nil
			c.ownedDone = nil
			c.ownedRecord = nil
			// Keep the dead identity as a tombstone. Repeated shutdown can prove
			// death without guessing from a leftover native socket symlink.
			return nil
		}
		escalate := c.clock.Since(start) >= 5*time.Second
		for _, member := range members {
			if escalate || !terminating[member.identity] {
				signal := unix.SIGTERM
				if escalate {
					signal = unix.SIGKILL
				}
				if err = codexSignalProcesses([]codexOwnedProcess{member}, signal); err != nil {
					break
				}
				terminating[member.identity] = true
			}
		}
		if err == nil {
			budget := 5 * time.Second
			if escalate {
				budget = 10 * time.Second
			}
			remaining := budget - c.clock.Since(start)
			if remaining <= 0 && escalate {
				err = xerrors.New("Codex process group did not stop")
			} else if remaining > 0 {
				err = c.waitProcesses(ctx, members, remaining)
			}
		}
		codexCloseProcesses(members)
		if err != nil {
			return err
		}
	}
}

// pidfd readiness wakes on any member's exit. The injected clock bounds graceful
// drain; an eventfd also wakes the kernel wait on timeout or context cancellation.
func (c *Codex) waitProcesses(ctx context.Context, members []codexOwnedProcess, duration time.Duration) error {
	fd, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err != nil {
		return err
	}
	var mu sync.Mutex
	closed := false
	wake := func() {
		mu.Lock()
		defer mu.Unlock()
		if !closed {
			_, _ = unix.Write(fd, []byte{1, 0, 0, 0, 0, 0, 0, 0})
		}
	}
	timer := c.clock.AfterFunc(duration, wake, "codex-shutdown-wait")
	stop := context.AfterFunc(ctx, wake)
	defer func() {
		timer.Stop()
		stop()
		mu.Lock()
		closed = true
		_ = unix.Close(fd)
		mu.Unlock()
	}()
	pollFD, err := codexPollDescriptor(fd)
	if err != nil {
		return err
	}
	poll := []unix.PollFd{pollFD}
	for _, member := range members {
		pollFD, err := codexPollDescriptor(member.fd)
		if err != nil {
			return err
		}
		poll = append(poll, pollFD)
	}
	for {
		_, err := unix.Poll(poll, -1)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		return ctx.Err()
	}
}
