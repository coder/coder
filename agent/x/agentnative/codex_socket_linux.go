//go:build linux

package agentnative

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// socketPeer inspects the actual listener, not the proxy process or a PID file.
// Codex publishes a rendezvous symlink to a short protected socket path.
func (c *Codex) socketPeer(ctx context.Context) (codexSocketPeer, error) {
	var peer codexSocketPeer
	path := filepath.Join(c.options.StateDir, "codex.sock")
	physical, err := filepath.EvalSymlinks(path)
	if errors.Is(err, os.ErrNotExist) {
		return peer, errCodexSocketClosed
	}
	if err != nil {
		return peer, errCodexOwnership
	}
	before, err := os.Stat(physical)
	if errors.Is(err, os.ErrNotExist) {
		return peer, errCodexSocketClosed
	}
	if err != nil || before.Mode()&os.ModeSocket == 0 {
		return peer, errCodexOwnership
	}
	ctx, cancel := codexTimeout(ctx, c.clock, time.Second, "codex-socket-peer")
	defer cancel()
	var dialer net.Dialer
	connection, err := dialer.DialContext(ctx, "unix", physical)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, unix.ECONNREFUSED) {
		return peer, errCodexSocketClosed
	}
	if err != nil {
		return peer, errCodexOwnership
	}
	defer connection.Close()
	uds, ok := connection.(*net.UnixConn)
	if !ok {
		return peer, errCodexOwnership
	}
	raw, err := uds.SyscallConn()
	if err != nil {
		return peer, errCodexOwnership
	}
	var cred *unix.Ucred
	var credentialErr error
	if err := raw.Control(func(fd uintptr) {
		// A socket descriptor is allocated by the kernel as a signed int.
		// #nosec G115 -- SyscallConn supplies a valid socket descriptor.
		cred, credentialErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil || credentialErr != nil || cred == nil || cred.Pid <= 1 || int64(cred.Uid) != int64(os.Geteuid()) {
		return peer, errCodexOwnership
	}
	identity, marker, err := codexProcessIdentity(int(cred.Pid))
	if err != nil || identity == nil {
		return peer, errCodexOwnership
	}
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) {
		return peer, errCodexOwnership
	}
	stat, ok := after.Sys().(*syscall.Stat_t)
	if !ok {
		return peer, errCodexOwnership
	}
	return codexSocketPeer{Process: *identity, Marker: marker, Device: stat.Dev, Inode: stat.Ino}, nil
}

func codexSocketMatches(record codexServerRecord, peer codexSocketPeer) bool {
	boot, namespace, err := codexProcessDomain()
	if err != nil || boot != record.Boot || namespace != record.Namespace || peer.Marker != record.Marker || peer.Process.Session != record.Leader.Session {
		return false
	}
	return peer.Process.PID != record.Leader.PID || peer.Process == record.Leader
}

// boundSocket is checked around proxy creation and before reusing a connection.
// A retired ownership record never grants authority over a replacement listener.
func (c *Codex) boundSocket(ctx context.Context) (codexSocketPeer, error) {
	peer, err := c.socketPeer(ctx)
	if err != nil {
		return peer, err
	}
	if err := c.finalizeServerIdentity(); err != nil {
		return peer, err
	}
	record, err := c.readServerRecord()
	if err != nil || record == nil || !codexSocketMatches(*record, peer) {
		return peer, errCodexOwnership
	}
	return peer, nil
}
