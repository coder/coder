// Package clientmeta identifies AI clients and request correlation metadata.
package clientmeta

import (
	"net/http"
	"strings"
)

// Client identifies an AI client application.
type Client string

const (
	ClientClaudeCode  Client = "Claude Code"
	ClientCodex       Client = "Codex"
	ClientZed         Client = "Zed"
	ClientCopilotVSC  Client = "GitHub Copilot (VS Code)"
	ClientCopilotCLI  Client = "GitHub Copilot (CLI)"
	ClientKilo        Client = "Kilo Code"
	ClientCoderAgents Client = "Coder Agents"
	ClientCrush       Client = "Charm Crush"
	ClientXum         Client = "Xum"
	ClientRoo         Client = "Roo Code"
	ClientCursor      Client = "Cursor"
	ClientOpenCode    Client = "OpenCode"
	ClientJunie       Client = "Junie"
	ClientUnknown     Client = "Unknown"
)

// GuessClient attempts to identify the client application from request headers.
func GuessClient(r *http.Request) Client {
	userAgent := strings.ToLower(r.UserAgent())
	originator := r.Header.Get("originator")

	switch {
	case strings.HasPrefix(userAgent, "xum/") || strings.HasPrefix(userAgent, "mux/"):
		return ClientXum
	case strings.HasPrefix(userAgent, "claude"):
		return ClientClaudeCode
	case strings.HasPrefix(userAgent, "codex"):
		return ClientCodex
	case strings.HasPrefix(userAgent, "zed/"):
		return ClientZed
	case strings.HasPrefix(userAgent, "githubcopilotchat/"):
		return ClientCopilotVSC
	case strings.HasPrefix(userAgent, "copilot/"):
		return ClientCopilotCLI
	case strings.HasPrefix(userAgent, "kilo-code/") || originator == "kilo-code":
		return ClientKilo
	case strings.HasPrefix(userAgent, "roo-code/") || originator == "roo-code":
		return ClientRoo
	case strings.HasPrefix(userAgent, "coder-agents/"):
		return ClientCoderAgents
	case strings.HasPrefix(userAgent, "charm crush/") || strings.HasPrefix(userAgent, "charm-crush/"):
		return ClientCrush
	case r.Header.Get("x-cursor-client-version") != "":
		return ClientCursor
	case strings.HasPrefix(userAgent, "opencode/"):
		return ClientOpenCode
	case strings.HasPrefix(userAgent, "junie:"):
		return ClientJunie
	default:
		return ClientUnknown
	}
}
