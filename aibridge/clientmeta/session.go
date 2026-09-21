package clientmeta

import (
	"bytes"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
)

var claudeCodePattern = regexp.MustCompile(`_session_(.+)$`)

// GuessSessionID attempts to retrieve a client session ID and restores a body
// that it reads.
func GuessSessionID(client Client, r *http.Request) *string {
	if sessionID := GuessSessionIDFromPayload(client, r, nil); client != ClientClaudeCode || sessionID != nil {
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
	return GuessSessionIDFromPayload(client, r, payload)
}

// GuessSessionIDFromPayload attempts to retrieve a client session ID without
// reading the request body.
func GuessSessionIDFromPayload(client Client, r *http.Request, payload []byte) *string {
	switch client {
	case ClientClaudeCode:
		if sid := cleanRef(r.Header.Get("X-Claude-Code-Session-Id")); sid != nil {
			return sid
		}
		userID := gjson.GetBytes(payload, "metadata.user_id")
		if userID.Type != gjson.String {
			return nil
		}
		raw := userID.String()
		if sessionID := gjson.Get(raw, "session_id"); sessionID.Exists() {
			return cleanRef(sessionID.String())
		}
		matches := claudeCodePattern.FindStringSubmatch(raw)
		if len(matches) < 2 {
			return nil
		}
		return cleanRef(matches[1])
	case ClientCodex:
		if sid := cleanRef(r.Header.Get("session-id")); sid != nil {
			return sid
		}
		return cleanRef(r.Header.Get("session_id"))
	case ClientXum:
		return cleanRef(r.Header.Get("X-Mux-Workspace-Id"))
	case ClientCopilotVSC:
		return cleanRef(r.Header.Get("x-interaction-id"))
	case ClientCopilotCLI:
		return cleanRef(r.Header.Get("X-Client-Session-Id"))
	case ClientKilo:
		return cleanRef(r.Header.Get("X-KILOCODE-TASKID"))
	case ClientCoderAgents:
		return cleanRef(r.Header.Get("X-Coder-Chat-Id"))
	case ClientOpenCode:
		if sid := cleanRef(r.Header.Get("X-OpenCode-Session")); sid != nil {
			return sid
		}
		return cleanRef(r.Header.Get("x-session-affinity"))
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
