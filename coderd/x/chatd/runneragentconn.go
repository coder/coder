package chatd

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// runnerAgentConn holds a chat runner's connection to one workspace agent,
// and that workspace agent's home directory, across generation steps.
//
// mu is held while acquiring a connection, so concurrent dials share one
// acquisition, and forget and close wait for it. No connection is released
// before close, including connections it stopped holding: a step may still
// use them, and the runner calls close only after its steps have ended.
type runnerAgentConn struct {
	mu       sync.Mutex
	closed   bool
	agentID  uuid.UUID
	conn     workspacesdk.AgentConn // Nil if none is held.
	home     string                 // Read through conn; empty until read.
	releases []func()               // Of every connection acquired.
}

// dial returns the held connection if it is to agentID and the workspace
// agent is not disconnected. Otherwise it acquires a connection through
// server and holds it. The release it returns does nothing.
func (r *runnerAgentConn) dial(ctx context.Context, server *Server, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, nil, xerrors.New("chat runner closed its workspace agent connections")
	}
	if r.conn != nil && r.agentID == agentID && !server.workspaceAgentDisconnected(ctx, agentID) {
		return r.conn, func() {}, nil
	}
	conn, release, err := server.agentConnFn(ctx, agentID)
	if err != nil {
		return nil, nil, err
	}
	if release != nil {
		r.releases = append(r.releases, release)
	}
	r.agentID, r.conn, r.home = agentID, conn, ""
	return conn, func() {}, nil
}

func (r *runnerAgentConn) forget(agentID uuid.UUID) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.agentID == agentID {
		r.agentID, r.conn, r.home = uuid.Nil, nil, ""
	}
}

func (r *runnerAgentConn) homeOf(conn workspacesdk.AgentConn) (string, bool) {
	if r == nil {
		return "", false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn != conn || r.home == "" {
		return "", false
	}
	return r.home, true
}

func (r *runnerAgentConn) setHome(conn workspacesdk.AgentConn, home string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn == conn {
		r.home = home
	}
}

// close releases every connection it acquired, and later dials fail. Call
// it only after every step has ended.
func (r *runnerAgentConn) close() {
	r.mu.Lock()
	releases := r.releases
	r.closed = true
	r.agentID, r.conn, r.home, r.releases = uuid.Nil, nil, "", nil
	r.mu.Unlock()
	for _, release := range releases {
		release()
	}
}
