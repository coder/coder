package clientmeta

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
// the client. We only attempt to retrieve sessions using methods recognized for
// the given client.
func GuessSessionID(client Client, r *http.Request) *string {
	sessionID, needsPayload := sessionIDFromInputs(client, r, nil)
	if sessionID != nil || !needsPayload {
		return sessionID
	}

	var payload []byte
	if r.Body != nil {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil
		}
		_ = r.Body.Close()
		payload = body
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	sessionID, _ = sessionIDFromInputs(client, r, payload)
	return sessionID
}

// GuessSessionIDFromPayload attempts to retrieve a client session ID without
// reading the request body.
func GuessSessionIDFromPayload(client Client, r *http.Request, payload []byte) *string {
	sessionID, _ := sessionIDFromInputs(client, r, payload)
	return sessionID
}

func sessionIDFromInputs(client Client, r *http.Request, payload []byte) (*string, bool) {
	switch client {
	case ClientClaudeCode:
		// Prefer the dedicated header (added in Claude Code v2.1.86+).
		if sid := cleanRef(r.Header.Get("X-Claude-Code-Session-Id")); sid != nil {
			return sid, false
		}
		// Fall back to extracting from the metadata.user_id field in the JSON body.
		// Newer format: JSON-encoded object with a "session_id" field.
		// Legacy format: "user_{sha256}_account_{id}_session_{uuid}"
		userID := gjson.GetBytes(payload, "metadata.user_id")
		if userID.Type != gjson.String {
			return nil, true
		}
		raw := userID.String()
		// Newer body format: user_id is a JSON-encoded object with a session_id field.
		if sessionID := gjson.Get(raw, "session_id"); sessionID.Exists() {
			return cleanRef(sessionID.String()), true
		}
		// Legacy body format: "user_{sha256}_account_{id}_session_{uuid}"
		matches := claudeCodePattern.FindStringSubmatch(raw)
		if len(matches) < 2 {
			return nil, true
		}
		return cleanRef(matches[1]), true
	case ClientCodex:
		// Codex renamed the header from "session_id" to "session-id" in
		// newer releases. Check the current name first, then fall back to
		// the legacy name for older Codex versions.
		if sid := cleanRef(r.Header.Get("session-id")); sid != nil {
			return sid, false
		}
		return cleanRef(r.Header.Get("session_id")), false
	case ClientXum:
		// Header name is intentionally still "X-Mux-Workspace-Id"; Xum keeps the
		// wire value from before the Mux rename.
		return cleanRef(r.Header.Get("X-Mux-Workspace-Id")), false
	case ClientCopilotVSC:
		// This does not map precisely to what we consider a session, but it's close enough.
		// Most other providers' equivalent of this would persist for the duration of a
		// conversation; it does seem to persist across an agentic loop though, which is
		// all we really need.
		//
		// There's also `vscode-sessionid` but that's persistent for the duration of the
		// VS Code window.
		return cleanRef(r.Header.Get("x-interaction-id")), false
	case ClientCopilotCLI:
		return cleanRef(r.Header.Get("X-Client-Session-Id")), false
	case ClientKilo:
		return cleanRef(r.Header.Get("X-KILOCODE-TASKID")), false
	case ClientCoderAgents:
		return cleanRef(r.Header.Get("X-Coder-Chat-Id")), false
	case ClientOpenCode:
		// Prefer X-OpenCode-Session (set by the OpenCode "Zen" provider).
		if sid := cleanRef(r.Header.Get("X-OpenCode-Session")); sid != nil {
			return sid, false
		}
		// Fall back to x-session-affinity (set by other providers).
		return cleanRef(r.Header.Get("x-session-affinity")), false
	case ClientZed:
		return nil, false // Zed does not send a session ID from Zed Agent or Text Thread.
	case ClientCrush:
		return nil, false // Crush does not send a session ID header.
	case ClientRoo:
		return nil, false // RooCode doesn't send a session ID.
	case ClientCursor:
		return nil, false // Cursor is not currently supported.
	default:
		return nil, false
	}
}

func cleanRef(str string) *string {
	str = strings.TrimSpace(str)
	if str == "" {
		return nil
	}
	return new(str)
}
