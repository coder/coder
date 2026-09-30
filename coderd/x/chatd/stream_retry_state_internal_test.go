package chatd

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
)

// TestStreamSnapshotRetryAfterTurnEnded checks that a turn which records a
// retry and then ends without a new generation attempt does not keep
// announcing that retry. The trigger only clears chats.retry_state when
// generation_attempt changes, so leaving running must clear it too.
// Otherwise the stream snapshot for a newly connected client emits a retry
// event after the status event, and the frontend shows a retry banner for
// a chat that is no longer running.
func TestStreamSnapshotRetryAfterTurnEnded(t *testing.T) {
	t.Parallel()

	const toolName = "browser"
	cases := []struct {
		name string
		// beforeAttempt runs after Acquire and before the generation
		// attempt that fails and records the retry.
		beforeAttempt func(tx *chatstate.Tx, modelID uuid.UUID) error
		end           func(tx *chatstate.Tx) error
		wantStat      database.ChatStatus
	}{
		{
			// Interrupt while waiting to retry an attempt that produced
			// no parts: FinishInterruption commits no partial messages.
			name: "InterruptedDuringRetryBackoff",
			end: func(tx *chatstate.Tx) error {
				if _, err := tx.Interrupt(chatstate.InterruptInput{}); err != nil {
					return err
				}
				_, err := tx.FinishInterruption(chatstate.FinishInterruptionInput{})
				return err
			},
			wantStat: database.ChatStatusWaiting,
		},
		{
			// The next loop iteration fails before a new attempt is
			// recorded and parks the chat in error.
			name: "ErrorAfterRetry",
			end: func(tx *chatstate.Tx) error {
				lastErr, err := json.Marshal(codersdk.ChatError{Message: "failed", Kind: codersdk.ChatErrorKindGeneric})
				if err != nil {
					return err
				}
				_, err = tx.FinishError(chatstate.FinishErrorInput{
					LastError: pqtype.NullRawMessage{RawMessage: lastErr, Valid: true},
				})
				return err
			},
			wantStat: database.ChatStatusError,
		},
		{
			// A committed step left a pending dynamic tool call, and
			// the chat parks in requires_action instead of retrying.
			name: "RequiresActionAfterRetry",
			beforeAttempt: func(tx *chatstate.Tx, modelID uuid.UUID) error {
				content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{{
					Type:       codersdk.ChatMessagePartTypeToolCall,
					ToolCallID: "call-1",
					ToolName:   toolName,
					Args:       json.RawMessage(`{}`),
				}})
				if err != nil {
					return err
				}
				_, err = tx.CommitStep(chatstate.CommitStepInput{
					Messages: []chatstate.Message{{
						Role:           database.ChatMessageRoleAssistant,
						Content:        content,
						Visibility:     database.ChatMessageVisibilityBoth,
						ContentVersion: chatprompt.CurrentContentVersion,
						ModelConfigID:  uuid.NullUUID{UUID: modelID, Valid: true},
					}},
				})
				return err
			},
			end: func(tx *chatstate.Tx) error {
				_, err := tx.EnterRequiresAction(chatstate.EnterRequiresActionInput{})
				return err
			},
			wantStat: database.ChatStatusRequiresAction,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, ps := dbtestutil.NewDB(t)
			ctx := chatdTestContext(t)

			user := dbgen.User(t, db, database.User{})
			org := dbgen.Organization(t, db, database.Organization{})
			dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
			dbgen.ChatProvider(t, db, database.ChatProvider{Provider: "openai", DisplayName: "OpenAI"})
			model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
				Model:          "gpt-4.1",
				DisplayName:    "gpt-4.1",
				OrganizationID: org.ID,
			}, func(p *database.InsertChatModelConfigParams) {
				p.Enabled = true
				p.IsDefault = true
			})
			dynamicTools, err := json.Marshal([]codersdk.DynamicTool{{
				Name:        toolName,
				Description: "test tool",
				InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
			}})
			require.NoError(t, err)
			created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
				OrganizationID:    org.ID,
				OwnerID:           user.ID,
				LastModelConfigID: model.ID,
				Title:             "retry",
				ClientType:        database.ChatClientTypeUi,
				DynamicTools:      pqtype.NullRawMessage{RawMessage: dynamicTools, Valid: true},
				InitialMessages: []chatstate.Message{{
					Role:           database.ChatMessageRoleUser,
					Content:        mustMarshalText(t, "hello"),
					Visibility:     database.ChatMessageVisibilityBoth,
					ContentVersion: chatprompt.CurrentContentVersion,
					CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
					ModelConfigID:  uuid.NullUUID{UUID: model.ID, Valid: true},
				}},
			})
			require.NoError(t, err)
			machine := chatstate.NewChatMachine(db, ps, created.Chat.ID)
			update := func(fn func(tx *chatstate.Tx) error) {
				t.Helper()
				require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
					return fn(tx)
				}))
			}

			// Worker acquires, starts attempt 1, the attempt fails with a
			// retryable error and the retry payload is recorded
			// (recordGenerationRetry).
			update(func(tx *chatstate.Tx) error {
				_, err := tx.Acquire(chatstate.AcquireInput{WorkerID: uuid.New(), RunnerID: uuid.New()})
				return err
			})
			if tc.beforeAttempt != nil {
				update(func(tx *chatstate.Tx) error {
					return tc.beforeAttempt(tx, model.ID)
				})
			}
			update(func(tx *chatstate.Tx) error {
				_, err := tx.RecordGenerationAttempt(chatstate.RecordGenerationAttemptInput{})
				return err
			})
			retry, err := json.Marshal(codersdk.ChatStreamRetry{Attempt: 1, DelayMs: 1000, Error: "rate limited", RetryingAt: time.Now()})
			require.NoError(t, err)
			var retryStateVersion int64
			update(func(tx *chatstate.Tx) error {
				recorded, err := tx.RecordRetryState(chatstate.RecordRetryStateInput{
					RetryState: pqtype.NullRawMessage{RawMessage: retry, Valid: true},
				})
				retryStateVersion = recorded.Chat.RetryStateVersion
				return err
			})
			require.Positive(t, retryStateVersion)
			update(tc.end)

			chat, err := db.GetChatByID(ctx, created.Chat.ID)
			require.NoError(t, err)
			require.Equal(t, tc.wantStat, chat.Status)
			require.False(t, chat.RetryState.Valid, "leaving running must clear the pending retry")
			// Every client-visible retry-state change advances
			// retry_state_version, so connected streams observe the clear.
			require.Greater(t, chat.RetryStateVersion, retryStateVersion)

			// A client connects: the bootstrap sync in stream_subscribe.go.
			loop := newStreamLoop(chat, db, slogtest.Make(t, nil), 0)
			events, _, changed, err := loop.syncDB(ctx)
			require.NoError(t, err)
			require.True(t, changed)
			types := make([]codersdk.ChatStreamEventType, 0, len(events))
			for _, event := range events {
				types = append(types, event.Type)
			}
			require.NotContains(t, types, codersdk.ChatStreamEventTypeRetry,
				"stream snapshot for a %s chat must not announce a pending retry; events: %v", chat.Status, types)
		})
	}
}
