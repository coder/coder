package chatstate

import (
	"database/sql"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	"github.com/coder/coder/v2/coderd/database"
)

// Message is the durable message input shape used by chatstate
// transitions. It is intentionally lower level than the SDK message
// request types: callers must produce a fully materialized message
// (parsed parts, calculated cost, resolved model config) before
// passing it in.
//
// The state machine never reshapes a Message except to attach the
// runtime `chat_id`.
type Message struct {
	Role                database.ChatMessageRole
	Content             pqtype.NullRawMessage
	Visibility          database.ChatMessageVisibility
	ModelConfigID       uuid.NullUUID
	CreatedBy           uuid.NullUUID
	ContentVersion      int16
	Compressed          bool
	InputTokens         sql.NullInt64
	OutputTokens        sql.NullInt64
	TotalTokens         sql.NullInt64
	ReasoningTokens     sql.NullInt64
	CacheCreationTokens sql.NullInt64
	CacheReadTokens     sql.NullInt64
	ContextLimit        sql.NullInt64
	TotalCostMicros     sql.NullInt64
	RuntimeMs           sql.NullInt64
	APIKeyID            sql.NullString
}

// toInsertParams converts a batch of Messages into the parallel-array
// shape required by `InsertChatMessages`. The returned struct has all
// arrays sized to len(messages).
//
// The chat ID is supplied by the caller because Message itself does
// not carry one (the chat machine already knows the chat).
func toInsertParams(chatID uuid.UUID, messages []Message) database.InsertChatMessagesParams {
	n := len(messages)
	params := database.InsertChatMessagesParams{
		ChatID:              chatID,
		CreatedBy:           make([]uuid.UUID, n),
		ModelConfigID:       make([]uuid.UUID, n),
		APIKeyID:            make([]string, n),
		Role:                make([]database.ChatMessageRole, n),
		Content:             make([]string, n),
		ContentVersion:      make([]int16, n),
		Visibility:          make([]database.ChatMessageVisibility, n),
		InputTokens:         make([]int64, n),
		OutputTokens:        make([]int64, n),
		TotalTokens:         make([]int64, n),
		ReasoningTokens:     make([]int64, n),
		CacheCreationTokens: make([]int64, n),
		CacheReadTokens:     make([]int64, n),
		ContextLimit:        make([]int64, n),
		Compressed:          make([]bool, n),
		TotalCostMicros:     make([]int64, n),
		RuntimeMs:           make([]int64, n),
	}
	for i, m := range messages {
		params.CreatedBy[i] = nullUUIDOrNil(m.CreatedBy)
		params.ModelConfigID[i] = nullUUIDOrNil(m.ModelConfigID)
		if m.APIKeyID.Valid {
			params.APIKeyID[i] = m.APIKeyID.String
		}
		params.Role[i] = m.Role
		if m.Content.Valid {
			params.Content[i] = string(m.Content.RawMessage)
		} else {
			// Use the JSON null literal; UNNEST + ::jsonb requires a
			// valid JSON value and the trigger leaves it untouched.
			params.Content[i] = "null"
		}
		params.ContentVersion[i] = m.ContentVersion
		params.Visibility[i] = m.Visibility
		params.InputTokens[i] = nullInt64Or(m.InputTokens, 0)
		params.OutputTokens[i] = nullInt64Or(m.OutputTokens, 0)
		params.TotalTokens[i] = nullInt64Or(m.TotalTokens, 0)
		params.ReasoningTokens[i] = nullInt64Or(m.ReasoningTokens, 0)
		params.CacheCreationTokens[i] = nullInt64Or(m.CacheCreationTokens, 0)
		params.CacheReadTokens[i] = nullInt64Or(m.CacheReadTokens, 0)
		params.ContextLimit[i] = nullInt64Or(m.ContextLimit, 0)
		params.Compressed[i] = m.Compressed
		params.TotalCostMicros[i] = nullInt64Or(m.TotalCostMicros, 0)
		params.RuntimeMs[i] = nullInt64Or(m.RuntimeMs, 0)
	}
	return params
}

func nullUUIDOrNil(u uuid.NullUUID) uuid.UUID {
	if u.Valid {
		return u.UUID
	}
	return uuid.Nil
}

func nullInt64Or(v sql.NullInt64, fallback int64) int64 {
	if v.Valid {
		return v.Int64
	}
	return fallback
}
