package agentnative

import (
	"context"
	"errors"
	"path/filepath"
	"regexp"
	"time"

	"golang.org/x/xerrors"
)

var codexUserAgent = regexp.MustCompile(`^[^\s/]+/0\.164\.0(?:\s|$)`)

type codexProcessRecord struct {
	PID        int    `json:"pid"`
	Group      int    `json:"group"`
	Session    int    `json:"session"`
	Start      string `json:"start"`
	Executable string `json:"executable"`
	Device     uint64 `json:"device"`
	Inode      uint64 `json:"inode"`
}

type codexServerRecord struct {
	Version   string             `json:"version"`
	Boot      string             `json:"boot"`
	Namespace string             `json:"namespace"`
	Marker    string             `json:"marker"`
	Socket    string             `json:"socket"`
	Leader    codexProcessRecord `json:"leader"`
}

type codexSocketPeer struct {
	Process codexProcessRecord
	Marker  string
	Device  uint64
	Inode   uint64
}

var errCodexSocketClosed = xerrors.New("native Codex socket is absent or closed")

var errCodexOwnership = xerrors.New("cannot prove Codex server process ownership; unverified processes were not signaled")

type codexVersionError struct{}

func (*codexVersionError) Error() string {
	return "native Codex must be exactly version 0.164.0 on Unix"
}

// ensure is called with lifecycle held. Native socket locking arbitrates
// competing launches. Never unlink or infer ownership from a socket/PID file.
//
//nolint:revive // start distinguishes lazy startup from strictly read-only attachment.
func (c *Codex) ensure(ctx context.Context, start bool) (codexRPC, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, xerrors.New("codex observer is closed")
	}
	connection := c.conn
	c.mu.Unlock()
	if connection != nil {
		if bound, ok := connection.(*codexBoundRPC); ok {
			peer, err := c.boundSocket(ctx)
			if err != nil || peer != bound.peer {
				return nil, errCodexOwnership
			}
		}
		return connection, nil
	}
	if err := c.privateDirectory(start); err != nil {
		return nil, err
	}
	ctx, cancel := codexTimeout(ctx, c.clock, 20*time.Second, "codex-readiness")
	defer cancel()
	probe := func() (codexRPC, error) {
		attempt, stop := codexTimeout(ctx, c.clock, time.Second, "codex-probe")
		defer stop()
		return c.connect(attempt)
	}
	connection, err := probe()
	if err == nil {
		return connection, nil
	}
	var incompatible *codexVersionError
	if !start || errors.As(err, &incompatible) || errors.Is(err, errCodexOwnership) {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	started := false
	if c.owned != nil {
		select {
		case <-c.ownedDone:
			c.owned = nil
			c.ownedDone = nil
		default:
		}
	}
	if c.owned == nil {
		if err := c.checkServerStart(ctx); err != nil {
			return nil, err
		}
		if err := c.startServer(); err != nil {
			return nil, err
		}
		started = true
	}
	ready := false
	defer func() {
		if started && !ready {
			// Only this invocation's child can be cleaned up on failed startup.
			_ = c.killOwnedGroup()
		}
	}()
	for {
		if err := ctx.Err(); err != nil {
			return nil, xerrors.New("codex app-server readiness was not confirmed")
		}
		connection, err = probe()
		if err == nil {
			if err := c.finalizeServerIdentity(); err != nil {
				_ = connection.close()
				return nil, err
			}
			ready = true
			return connection, nil
		}
		if errors.As(err, &incompatible) {
			return nil, err
		}
		select {
		case <-c.ownedDone:
			return nil, xerrors.New("codex app-server exited before readiness")
		default:
		}
		timer := c.clock.NewTimer(100*time.Millisecond, "codex-poll")
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, xerrors.New("codex app-server readiness was not confirmed")
		case <-timer.C:
		}
	}
}

func (c *Codex) connect(ctx context.Context) (codexRPC, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, xerrors.New("codex observer is closed")
	}
	c.generation++
	generation := c.generation
	c.mu.Unlock()
	connection, err := c.dial(ctx, func(m codexMessage) { c.receive(generation, m) }, func() { c.disconnected(generation) })
	if err != nil {
		return nil, err
	}
	ready := false
	defer func() {
		if !ready {
			_ = connection.close()
		}
	}()
	var initialized struct {
		UserAgent      string `json:"userAgent"`
		PlatformFamily string `json:"platformFamily"`
		CodexHome      string `json:"codexHome"`
	}
	err = connection.call(ctx, "initialize", map[string]any{
		"clientInfo":   map[string]any{"name": "coder_native_agent", "title": "Coder Native Agent", "version": "0.1.0"},
		"capabilities": map[string]any{"experimentalApi": true, "requestAttestation": false},
	}, &initialized)
	if err != nil {
		return nil, err
	}
	if !codexUserAgent.MatchString(initialized.UserAgent) || initialized.PlatformFamily != "unix" || !filepath.IsAbs(initialized.CodexHome) {
		return nil, &codexVersionError{}
	}
	if err := connection.notify(ctx, "initialized"); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || generation != c.generation {
		return nil, xerrors.New("codex connection lost during initialization")
	}
	c.conn = connection
	c.seenServer = true
	ready = true
	return connection, nil
}
