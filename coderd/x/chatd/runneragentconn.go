package chatd

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// runnerAgentConn holds a chat runner's connection to one workspace agent,
// and that workspace agent's home directory, across generation steps. Steps
// dial through it, so they reuse the connection instead of acquiring one per
// step.
//
// It releases no connection before close, including connections it stopped
// holding: a step may still use them, and the runner calls close only after
// its steps have ended.
type runnerAgentConn struct {
	mu       sync.Mutex
	agentID  uuid.UUID
	conn     workspacesdk.AgentConn // Nil if none is held.
	home     string                 // Of agentID; empty until read.
	releases []func()               // Of every connection acquired.
}

// dial returns the held connection if it is to agentID and the workspace
// agent is not disconnected. Otherwise it acquires a connection through
// server and holds it. The release it returns does nothing.
func (r *runnerAgentConn) dial(ctx context.Context, server *Server, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
	r.mu.Lock()
	conn := r.conn
	if r.agentID != agentID {
		conn = nil
	}
	r.mu.Unlock()
	if conn != nil {
		if !server.workspaceAgentDisconnected(ctx, agentID) {
			return conn, func() {}, nil
		}
		r.forget(agentID)
	}

	conn, release, err := server.agentConnFn(ctx, agentID)
	if err != nil {
		return nil, nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if release != nil {
		r.releases = append(r.releases, release)
	}
	r.agentID = agentID
	r.conn = conn
	r.home = ""
	return conn, func() {}, nil
}

// forget stops holding the connection to agentID, so the next dial to it
// acquires a new one.
func (r *runnerAgentConn) forget(agentID uuid.UUID) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.agentID == agentID {
		r.agentID = uuid.Nil
		r.conn = nil
		r.home = ""
	}
}

// homeOf returns the home directory of agentID if it holds a connection to
// agentID and has read it.
func (r *runnerAgentConn) homeOf(agentID uuid.UUID) (string, bool) {
	if r == nil {
		return "", false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.agentID != agentID || r.home == "" {
		return "", false
	}
	return r.home, true
}

// setHome records the home directory read through conn, if conn is the held
// connection.
func (r *runnerAgentConn) setHome(conn workspacesdk.AgentConn, home string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn != nil && r.conn == conn {
		r.home = home
	}
}

// close releases every connection it acquired. Call it only after every step
// has ended.
func (r *runnerAgentConn) close() {
	r.mu.Lock()
	releases := r.releases
	r.releases = nil
	r.agentID = uuid.Nil
	r.conn = nil
	r.home = ""
	r.mu.Unlock()
	for _, release := range releases {
		release()
	}
}
