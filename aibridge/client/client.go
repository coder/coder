// Package client identifies AI clients and request correlation metadata.
package client

import (
	"net/http"
	"strings"
)

// Type identifies an AI client application.
type Type string

const (
	// Possible values for the "client" field in interception records.
	// Must be kept in sync with documentation: https://github.com/coder/coder/blob/3cf867f84aa32d2febf7a26dc7e52be6beb8a2ac/docs/ai-coder/ai-gateway/monitoring.md?plain=1#L47-L57
	ClaudeCode  Type = "Claude Code"
	Codex       Type = "Codex"
	Zed         Type = "Zed"
	CopilotVSC  Type = "GitHub Copilot (VS Code)"
	CopilotCLI  Type = "GitHub Copilot (CLI)"
	Kilo        Type = "Kilo Code"
	CoderAgents Type = "Coder Agents"
	Crush       Type = "Charm Crush"
	Xum         Type = "Xum"
	Roo         Type = "Roo Code"
	Cursor      Type = "Cursor"
	OpenCode    Type = "OpenCode"
	Junie       Type = "Junie"
	Unknown     Type = "Unknown"
)

// GuessClient attempts to guess the client application from the request headers.
// Not all clients set proper user agent headers, so this is a best-effort approach.
// Based on https://github.com/coder/aibridge/issues/20#issuecomment-3769444101.
func GuessClient(r *http.Request) Type {
	userAgent := strings.ToLower(r.UserAgent())
	originator := r.Header.Get("originator")

	// Must be kept in sync with documentation: https://github.com/coder/coder/blob/3cf867f84aa32d2febf7a26dc7e52be6beb8a2ac/docs/ai-coder/ai-gateway/monitoring.md?plain=1#L47-L57
	switch {
	case strings.HasPrefix(userAgent, "xum/") || strings.HasPrefix(userAgent, "mux/"):
		// Mux was renamed to Xum; "mux/" is kept for older, unupgraded clients.
		return Xum
	case strings.HasPrefix(userAgent, "claude"):
		return ClaudeCode
	case strings.HasPrefix(userAgent, "codex"):
		return Codex
	case strings.HasPrefix(userAgent, "zed/"):
		return Zed
	case strings.HasPrefix(userAgent, "githubcopilotchat/"):
		return CopilotVSC
	case strings.HasPrefix(userAgent, "copilot/"):
		return CopilotCLI
	case strings.HasPrefix(userAgent, "kilo-code/") || originator == "kilo-code":
		return Kilo
	case strings.HasPrefix(userAgent, "roo-code/") || originator == "roo-code":
		return Roo
	case strings.HasPrefix(userAgent, "coder-agents/"):
		return CoderAgents
	case strings.HasPrefix(userAgent, "charm crush/") || strings.HasPrefix(userAgent, "charm-crush/"):
		return Crush
	case r.Header.Get("x-cursor-client-version") != "":
		return Cursor
	case strings.HasPrefix(userAgent, "opencode/"):
		return OpenCode
	case strings.HasPrefix(userAgent, "junie:"):
		return Junie
	}
	return Unknown
}
