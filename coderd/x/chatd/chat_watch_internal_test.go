package chatd

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/codersdk"
)

const chatWatchProjectionPayloadBudgetBytes = 6000

func TestChatWatchEventUsesBoundedSummary(t *testing.T) {
	t.Parallel()

	parts := make([]codersdk.ChatMessagePart, 0, 40)
	for range 20 {
		parts = append(parts,
			codersdk.ChatMessagePart{
				Type:            codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath: "/workspace/" + strings.Repeat("huge-context/", 20) + "AGENTS.md",
			},
			codersdk.ChatMessagePart{
				Type:             codersdk.ChatMessagePartTypeSkill,
				SkillName:        "huge-context-skill",
				SkillDescription: strings.Repeat("skill description ", 80),
			},
		)
	}
	lastInjectedContext, err := json.Marshal(parts)
	require.NoError(t, err)

	summary := "latest summary"
	now := time.Now().UTC()
	chat := database.Chat{
		ID:                uuid.New(),
		OrganizationID:    uuid.New(),
		OwnerID:           uuid.New(),
		LastModelConfigID: uuid.New(),
		Title:             "oversized context",
		Status:            database.ChatStatusRunning,
		CreatedAt:         now,
		UpdatedAt:         now,
		LastTurnSummary:   sql.NullString{String: summary, Valid: true},
		LastInjectedContext: pqtype.NullRawMessage{
			RawMessage: lastInjectedContext,
			Valid:      true,
		},
	}

	fullPayload, err := json.Marshal(struct {
		Kind codersdk.ChatWatchEventKind `json:"kind"`
		Chat codersdk.Chat               `json:"chat"`
	}{
		Kind: codersdk.ChatWatchEventKindStatusChange,
		Chat: db2sdk.Chat(chat, nil, nil),
	})
	require.NoError(t, err)
	require.Greater(t, len(fullPayload), chatWatchProjectionPayloadBudgetBytes)

	pubsub := &chatWatchCapturePubsub{}
	require.NoError(t, publishChatWatchEvent(pubsub, chat, codersdk.ChatWatchEventKindStatusChange))
	require.Equal(t, coderdpubsub.ChatWatchEventChannel(chat.OwnerID), pubsub.channel)
	require.Less(t, len(pubsub.payload), chatWatchProjectionPayloadBudgetBytes)
	require.NotContains(t, string(pubsub.payload), "last_injected_context")
	require.NotContains(t, string(pubsub.payload), "huge-context")

	var event codersdk.ChatWatchEvent
	require.NoError(t, json.Unmarshal(pubsub.payload, &event))
	require.Equal(t, chat.ID, event.Chat.ID)
	require.Equal(t, chat.Title, event.Chat.Title)
	require.Equal(t, codersdk.ChatStatusRunning, event.Chat.Status)
	require.Equal(t, chat.LastModelConfigID, event.Chat.LastModelConfigID)
	require.Equal(t, summary, *event.Chat.LastTurnSummary)

	diffStatus := db2sdk.ChatSummary(chat, &database.ChatDiffStatus{
		ChatID:       chat.ID,
		ChangedFiles: 3,
	})
	require.NotNil(t, diffStatus.DiffStatus)
	require.Equal(t, int32(3), diffStatus.DiffStatus.ChangedFiles)
}

type chatWatchCapturePubsub struct {
	channel string
	payload []byte
}

func (p *chatWatchCapturePubsub) Publish(channel string, payload []byte) error {
	p.channel = channel
	p.payload = append([]byte(nil), payload...)
	return nil
}

func (p *chatWatchCapturePubsub) SubscribeWithErr(string, dbpubsub.ListenerWithErr) (func(), error) {
	return func() {}, nil
}
