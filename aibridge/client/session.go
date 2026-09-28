package client

import (
	"bytes"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
)

var claudeCodePattern = regexp.MustCompile(`_session_(.+)$`) // Legacy format: save compilation on each call.

// GuessSessionID attempts to retrieve a session ID which may have been sent by
// the client. If Claude Code has no usable session header, it reads and restores
// r.Body to inspect the payload. Other clients use only request headers.
func GuessSessionID(client Type, r *http.Request) *string {
	sessionID := GuessSessionIDFromPayload(client, r, nil)
	if sessionID != nil || client != ClaudeCode || r.Body == nil {
		return sessionID
	}

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		return nil
	}
	_ = r.Body.Close()

	// Restore the request body for the interceptor.
	r.Body = io.NopCloser(bytes.NewReader(payload))
	return GuessSessionIDFromPayload(client, r, payload)
}

// GuessSessionIDFromPayload retrieves a session ID from recognized request
// headers and a pre-read payload, without reading, closing, or replacing r.Body.
// A nil or empty payload permits only header-based lookup.
func GuessSessionIDFromPayload(client Type, r *http.Request, payload []byte) *string {
	switch client {
	case ClaudeCode:
		// Prefer the dedicated header (added in Claude Code v2.1.86+).
		if sid := cleanRef(r.Header.Get("X-Claude-Code-Session-Id")); sid != nil {
			return sid
		}

		// Fall back to extracting from the metadata.user_id field in the JSON body.
		// Newer format:  JSON-encoded object with a "session_id" field.
		// Legacy format: "user_{sha256}_account_{id}_session_{uuid}"
		userID := gjson.GetBytes(payload, "metadata.user_id")
		if userID.Type != gjson.String {
			return nil
		}

		raw := userID.String()

		// Newer body format: user_id is a JSON-encoded object with a session_id field.
		if sessionID := gjson.Get(raw, "session_id"); sessionID.Exists() {
			return cleanRef(sessionID.String())
		}

		// Legacy body format: "user_{sha256}_account_{id}_session_{uuid}"
		matches := claudeCodePattern.FindStringSubmatch(raw)
		if len(matches) < 2 {
			return nil
		}
		return cleanRef(matches[1])
	case Codex:
		// Codex renamed the header from "session_id" to "session-id" in
		// newer releases. Check the current name first, then fall back to
		// the legacy name for older Codex versions.
		if sid := cleanRef(r.Header.Get("session-id")); sid != nil {
			return sid
		}
		return cleanRef(r.Header.Get("session_id"))
	case Xum:
		// Header name is intentionally still "X-Mux-Workspace-Id"; Xum keeps the
		// wire value from before the Mux rename.
		return cleanRef(r.Header.Get("X-Mux-Workspace-Id"))
	case Zed:
		return nil // Zed does not send a session ID from Zed Agent or Text Thread.
	case CopilotVSC:
		// This does not map precisely to what we consider a session, but it's close enough.
		// Most other providers' equivalent of this would persist for the duration of a
		// conversation; it does seem to persist across an agentic loop though, which is
		// all we really need.
		//
		// There's also `vscode-sessionid` but that's persistent for the duration of the
		// VS Code window.
		return cleanRef(r.Header.Get("x-interaction-id"))
	case CopilotCLI:
		return cleanRef(r.Header.Get("X-Client-Session-Id"))
	case Kilo:
		return cleanRef(r.Header.Get("X-KILOCODE-TASKID"))
	case CoderAgents:
		return cleanRef(r.Header.Get("X-Coder-Chat-Id"))
	case OpenCode:
		// Prefer X-OpenCode-Session (set by the OpenCode "Zen" provider).
		if sid := cleanRef(r.Header.Get("X-OpenCode-Session")); sid != nil {
			return sid
		}
		// Fall back to x-session-affinity (set by other providers).
		return cleanRef(r.Header.Get("x-session-affinity"))
	case Crush:
		return nil // Crush does not send a session ID header.
	case Roo:
		return nil // RooCode doesn't send a session ID.
	case Cursor:
		return nil // Cursor is not currently supported.
	default:
		return nil
	}
}

func cleanRef(str string) *string {
	str = strings.TrimSpace(str)
	if str == "" {
		return nil
	}

	return new(str)
}
