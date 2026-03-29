package coderd

import "strings"

// chatAgentSuffix is the naming convention used to identify
// chat-designated infrastructure agents that should be hidden from REST
// API responses.
const chatAgentSuffix = "-coderd-chat"

// isChatAgent reports whether the given agent name matches the
// chat-agent naming convention.
func isChatAgent(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), chatAgentSuffix)
}
