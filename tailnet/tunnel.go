package tailnet

import "github.com/google/uuid"

type TunnelAuth interface {
	Authorize(dst uuid.UUID) bool
}

// SingleTailnetTunnelAuth allows all tunnels, since Coderd and wsproxy are allowed to initiate a tunnel to any agent
type SingleTailnetTunnelAuth struct{}

func (SingleTailnetTunnelAuth) Authorize(uuid.UUID) bool {
	return true
}

// ClientTunnelAuth allows connecting to a single, given agent
type ClientTunnelAuth struct {
	AgentID uuid.UUID
}

func (c ClientTunnelAuth) Authorize(dst uuid.UUID) bool {
	return c.AgentID == dst
}

// AgentTunnelAuth disallows all tunnels, since agents are not allowed to initiate their own tunnels
type AgentTunnelAuth struct{}

func (AgentTunnelAuth) Authorize(uuid.UUID) bool {
	return false
}
