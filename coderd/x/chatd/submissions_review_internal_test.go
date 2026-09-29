package chatd

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestExactTurnSurvivesSyntheticMessages(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)
	owner := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	provider := dbgen.ChatProvider(t, db, database.ChatProvider{Provider: "openai"})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{OrganizationID: org.ID, AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true}})
	chat := dbgen.Chat(t, db, database.Chat{OrganizationID: org.ID, OwnerID: owner.ID, LastModelConfigID: model.ID})
	original := dbgen.ChatMessage(t, db, database.ChatMessage{ChatID: chat.ID, Role: database.ChatMessageRoleUser, Visibility: database.ChatMessageVisibilityBoth, Content: pqtype.NullRawMessage{RawMessage: json.RawMessage("[]"), Valid: true}})
	selection := codersdk.ChatExactSettings{ModelConfigID: uuid.New(), Provider: "openai", Model: "original"}
	raw, err := json.Marshal(selection)
	require.NoError(t, err)
	requestID := uuid.New()
	_, err = db.InsertChatSubmission(ctx, database.InsertChatSubmissionParams{ID: uuid.New(), OrganizationID: org.ID, OwnerID: owner.ID, ActorID: owner.ID, RequestID: requestID, InputDigest: make([]byte, 32), Kind: "message", ChatID: chat.ID, Settings: raw})
	require.NoError(t, err)
	_, err = db.CompleteChatSubmission(ctx, database.CompleteChatSubmissionParams{OrganizationID: org.ID, ActorID: owner.ID, RequestID: requestID, MessageID: sql.NullInt64{Int64: original.ID, Valid: true}})
	require.NoError(t, err)
	server := &Server{db: db}
	changed := resolvedModelCall{dbConfig: database.ChatModelConfig{ID: uuid.New()}, resolvedProvider: "openai", resolvedModel: "changed"}
	assertSelection := func() {
		raw, err := db.GetLatestChatSubmissionSettings(ctx, chat.ID)
		require.NoError(t, err)
		var got codersdk.ChatExactSettings
		require.NoError(t, json.Unmarshal(raw, &got))
		require.Equal(t, selection, got)
		require.ErrorContains(t, server.validateExactTurn(ctx, chat.ID, changed), "exact model selection changed")
	}
	appendMessages := func(messages []chatstate.Message) {
		for _, m := range messages {
			dbgen.ChatMessage(t, db, database.ChatMessage{ChatID: chat.ID, Role: m.Role, Visibility: m.Visibility, Content: m.Content, Compressed: m.Compressed})
		}
	}
	hooks, err := chathooks.EventMessages(&chathooks.Result{ModelContext: "hook context"}, uuid.Nil)
	require.NoError(t, err)
	appendMessages(hooks)
	assertSelection()
	compacted, err := buildCompactionMessages(buildCompactionMessagesInput{toolCallID: "compact", compaction: compactionOutcome{SystemSummary: "summary", SummaryReport: "report"}, pendingUserMessages: []database.ChatMessage{original}})
	require.NoError(t, err)
	appendMessages(compacted.Messages)
	assertSelection()
	// A newer genuine legacy user turn intentionally ends the previous guarantee.
	dbgen.ChatMessage(t, db, database.ChatMessage{ChatID: chat.ID, Role: database.ChatMessageRoleUser, Visibility: database.ChatMessageVisibilityBoth, Content: pqtype.NullRawMessage{RawMessage: json.RawMessage("[]"), Valid: true}})
	require.NoError(t, server.validateExactTurn(ctx, chat.ID, changed))
}
