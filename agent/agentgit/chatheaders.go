package agentgit

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// ExtractChatContext reads chat identity headers from the request.
// Returns zero values if headers are absent (non-chat request).
func ExtractChatContext(r *http.Request) (chatID uuid.UUID, ancestorIDs []uuid.UUID, ok bool) {
	raw := r.Header.Get(workspacesdk.CoderChatIDHeader)
	if raw == "" {
		return uuid.Nil, nil, false
	}
	chatID, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, nil, false
	}
	rawAncestors := r.Header.Get(workspacesdk.CoderAncestorChatIDsHeader)
	if rawAncestors != "" {
		var ids []string
		if err := json.Unmarshal([]byte(rawAncestors), &ids); err == nil {
			for _, s := range ids {
				if id, err := uuid.Parse(s); err == nil {
					ancestorIDs = append(ancestorIDs, id)
				}
			}
		}
	}
	return chatID, ancestorIDs, true
}
