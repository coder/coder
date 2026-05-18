package chattest

import (
	"encoding/json"

	"github.com/sqlc-dev/pqtype"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// ChatMessageWithParts returns a database chat message whose content is the
// JSON encoding of the provided SDK message parts.
func ChatMessageWithParts(parts []codersdk.ChatMessagePart) database.ChatMessage {
	raw, _ := json.Marshal(parts)
	return database.ChatMessage{
		Content: pqtype.NullRawMessage{RawMessage: raw, Valid: true},
	}
}
