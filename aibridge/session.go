package aibridge

import (
	"bytes"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"

	"github.com/coder/coder/v2/aibridge/utils"
)

var claudeCodePattern = regexp.MustCompile(`_session_(.+)$`) // Legacy format: save compilation on each call.

// GuessSessionID attempts to retrieve a session ID which may have been sent by
// the client. We only attempt to retrieve sessions using methods recognized for
// the given client.
func GuessSessionID(client Client, r *http.Request) *string {
	switch client {
	case ClientClaudeCode:
		// Prefer the dedicated header (added in Claude Code v2.1.86+).
		if sid := cleanRef(r.Header.Get("X-Claude-Code-Session-Id")); sid != nil {
			return sid
		}

		// Fall back to extracting from the metadata.user_id field in the JSON body.
		// Newer format:  JSON-encoded object with a "session_id" field.
		// Legacy format: "user_{sha256}_account_{id}_session_{uuid}"
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			return nil
		}
		_ = r.Body.Close()

		// Restore the request body.
		r.Body = io.NopCloser(bytes.NewReader(payload))
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
	case ClientCodex:
		return cleanRef(r.Header.Get("session_id"))
	case ClientMux:
		return cleanRef(r.Header.Get("X-Mux-Workspace-Id"))
	case ClientZed:
		return nil // Zed does not send a session ID from Zed Agent or Text Thread.
	case ClientCopilotVSC:
		// This does not map precisely to what we consider a session, but it's close enough.
		// Most other providers' equivalent of this would persist for the duration of a
		// conversation; it does seem to persist across an agentic loop though, which is
		// all we really need.
		//
		// There's also `vscode-sessionid` but that's persistent for the duration of the
		// VS Code window.
		return cleanRef(r.Header.Get("x-interaction-id"))
	case ClientCopilotCLI:
		return cleanRef(r.Header.Get("X-Client-Session-Id"))
	case ClientKilo:
		return cleanRef(r.Header.Get("X-KILOCODE-TASKID"))
	case ClientCoderAgents:
		return cleanRef(r.Header.Get("X-Coder-Chat-Id"))
	case ClientCrush:
		return nil // Crush does not send a session ID header.
	case ClientRoo:
		return nil // RooCode doesn't send a session ID.
	case ClientCursor:
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

	return utils.PtrTo(str)
}
