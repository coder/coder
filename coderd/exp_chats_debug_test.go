package coderd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// driveChatToError transitions the chat from running to error state.
func driveChatToError(ctx context.Context, t *testing.T, api *coderd.API, chatID uuid.UUID, msg string) {
	t.Helper()
	chatdCtx := dbauthz.AsChatd(ctx) //nolint:gocritic
	errPayload, err := json.Marshal(map[string]string{"message": msg})
	require.NoError(t, err)
	machine := chatstate.NewChatMachine(api.Database, api.Pubsub, chatID)
	require.NoError(t, machine.Update(chatdCtx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.FinishError(chatstate.FinishErrorInput{
			LastError: pqtype.NullRawMessage{RawMessage: errPayload, Valid: true},
		})
		return err
	}))
}

// addQueuedMessage puts a message on the running chat's queue (R0 → R1).
func addQueuedMessage(ctx context.Context, t *testing.T, api *coderd.API, chat codersdk.Chat) {
	t.Helper()
	chatdCtx := dbauthz.AsChatd(ctx) //nolint:gocritic
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText("queue me")})
	require.NoError(t, err)
	machine := chatstate.NewChatMachine(api.Database, api.Pubsub, chat.ID)
	require.NoError(t, machine.Update(chatdCtx, func(tx *chatstate.Tx, _ database.Store) error {
		_, sendErr := tx.SendMessage(chatstate.SendMessageInput{
			Message: chatstate.Message{
				Role:           database.ChatMessageRoleUser,
				Content:        content,
				Visibility:     database.ChatMessageVisibilityBoth,
				ModelConfigID:  uuid.NullUUID{UUID: chat.LastModelConfigID, Valid: true},
				ContentVersion: chatprompt.CurrentContentVersion,
			},
			BusyBehavior: chatstate.BusyBehaviorQueue,
		})
		return sendErr
	}))
}

// getDebugSnapshot fetches the debug snapshot for a chat and returns the decoded map.
func getDebugSnapshot(t *testing.T, client *codersdk.ExperimentalClient, chatID uuid.UUID) map[string]any {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	resp, err := client.Request(ctx, http.MethodGet,
		"/api/experimental/chats/"+chatID.String()+"/debug/snapshot", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out
}

func dbSnap(snap map[string]any) map[string]any { return snap["database"].(map[string]any) }
func rtSnap(snap map[string]any) map[string]any { return snap["runtime"].(map[string]any) }

// createDebugChat creates a chat with the minimum required fields for debug
// snapshot tests. It also sets up the model config prerequisite.
func createDebugChat(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, orgID uuid.UUID) codersdk.Chat {
	t.Helper()
	_ = createChatModelConfig(t, client)
	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: orgID,
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "hello",
		}},
	})
	require.NoError(t, err)
	return chat
}

// TestGetChatDebugSnapshot_ExecutionState verifies execution_state for W/R0/E0/R1.
func TestGetChatDebugSnapshot_ExecutionState(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		drive      func(ctx context.Context, t *testing.T, api *coderd.API, chat codersdk.Chat)
		wantState  string
		wantStatus string
		wantQueue  float64
	}{
		{name: "R0", wantState: "R0", wantStatus: "running"},
		{
			name: "W",
			drive: func(ctx context.Context, t *testing.T, api *coderd.API, chat codersdk.Chat) {
				driveChatToWaiting(ctx, t, api, chat.ID)
			},
			wantState: "W", wantStatus: "waiting",
		},
		{
			name: "E0",
			drive: func(ctx context.Context, t *testing.T, api *coderd.API, chat codersdk.Chat) {
				driveChatToError(ctx, t, api, chat.ID, "test error")
			},
			wantState: "E0", wantStatus: "error",
		},
		{
			name: "R1",
			drive: func(ctx context.Context, t *testing.T, api *coderd.API, chat codersdk.Chat) {
				addQueuedMessage(ctx, t, api, chat)
			},
			wantState: "R1", wantStatus: "running", wantQueue: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			client, api := newChatClientWithAPI(t, withChatWorkerDisabled)
			firstUser := coderdtest.CreateFirstUser(t, client.Client)

			chat := createDebugChat(ctx, t, client, firstUser.OrganizationID)
			if tc.drive != nil {
				tc.drive(ctx, t, api, chat)
			}

			snap := getDebugSnapshot(t, client, chat.ID)
			require.Equal(t, tc.wantState, snap["execution_state"])
			db := dbSnap(snap)
			require.Equal(t, tc.wantStatus, db["status"])
			require.Equal(t, tc.wantQueue, db["queue_depth"])
			if tc.name == "E0" {
				// last_error is a JSON object; verify it contains the expected message.
				lastErrObj, ok := db["last_error"].(map[string]any)
				require.True(t, ok, "last_error should be a JSON object")
				require.Equal(t, "test error", lastErrObj["message"])
			}
		})
	}
}

// TestGetChatDebugSnapshot_MessageStats verifies the message_stats breakdown.
func TestGetChatDebugSnapshot_MessageStats(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	client, api := newChatClientWithAPI(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)

	chat := createDebugChat(ctx, t, client, firstUser.OrganizationID)

	// Commit 2 assistant + 1 tool message.
	chatdCtx := dbauthz.AsChatd(ctx) //nolint:gocritic
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText("hi")})
	require.NoError(t, err)
	machine := chatstate.NewChatMachine(api.Database, api.Pubsub, chat.ID)
	require.NoError(t, machine.Update(chatdCtx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.CommitStep(chatstate.CommitStepInput{
			Messages: []chatstate.Message{
				{Role: database.ChatMessageRoleAssistant, Content: content, Visibility: database.ChatMessageVisibilityBoth, ContentVersion: chatprompt.CurrentContentVersion},
				{Role: database.ChatMessageRoleTool, Content: content, Visibility: database.ChatMessageVisibilityBoth, ContentVersion: chatprompt.CurrentContentVersion},
				{Role: database.ChatMessageRoleAssistant, Content: content, Visibility: database.ChatMessageVisibilityBoth, ContentVersion: chatprompt.CurrentContentVersion},
			},
		})
		return err
	}))

	snap := getDebugSnapshot(t, client, chat.ID)
	stats := dbSnap(snap)["message_stats"].(map[string]any)
	byRole := stats["by_role"].(map[string]any)
	// chatd inserts system + user messages on creation; CommitStep adds 2 assistant + 1 tool.
	// Assert total is non-zero and deleted is zero; verify the roles we explicitly inserted.
	require.Greater(t, stats["total"].(float64), float64(0))
	require.EqualValues(t, 0, stats["deleted"])
	require.EqualValues(t, 2, byRole["assistant"])
	require.EqualValues(t, 1, byRole["tool"])
}

// TestGetChatDebugSnapshot_Heartbeat verifies heartbeat presence and staleness.
func TestGetChatDebugSnapshot_Heartbeat(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	mClock := quartz.NewMock(t)
	mClock.Set(time.Now())
	client, db, api := newChatClientWithAPIAndDatabase(t, withChatWorkerDisabled, func(o *coderdtest.Options) {
		o.Clock = mClock
	})
	firstUser := coderdtest.CreateFirstUser(t, client.Client)

	chat := createDebugChat(ctx, t, client, firstUser.OrganizationID)

	// No heartbeat initially.
	snap := getDebugSnapshot(t, client, chat.ID)
	require.Nil(t, dbSnap(snap)["heartbeat"])

	// Acquire ownership (sets worker_id + runner_id) then upsert a heartbeat.
	runnerID := uuid.New()
	chatdCtx := dbauthz.AsChatd(ctx) //nolint:gocritic
	machine := chatstate.NewChatMachine(api.Database, api.Pubsub, chat.ID)
	require.NoError(t, machine.Update(chatdCtx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.Acquire(chatstate.AcquireInput{
			WorkerID: uuid.New(),
			RunnerID: runnerID,
		})
		return err
	}))
	require.NoError(t, db.UpsertChatHeartbeat(chatdCtx, database.UpsertChatHeartbeatParams{
		ChatID:   chat.ID,
		RunnerID: runnerID,
	}))

	// Resync the mock clock to wall time immediately before reading the
	// heartbeat: earlier setup (user/chat creation) can take a while in a
	// slow test environment, and the heartbeat timestamp is written by
	// Postgres using its own wall clock, not the mock clock.
	mClock.Set(time.Now())

	snap = getDebugSnapshot(t, client, chat.ID)
	hb := dbSnap(snap)["heartbeat"].(map[string]any)
	require.Equal(t, runnerID.String(), hb["runner_id"])
	require.GreaterOrEqual(t, hb["age_seconds"].(float64), float64(0))
	require.False(t, hb["is_stale"].(bool))

	// Advance the clock past the staleness threshold and verify is_stale flips.
	mClock.Advance((chatstate.HeartbeatStaleSeconds + 1) * time.Second)

	snap = getDebugSnapshot(t, client, chat.ID)
	hb = dbSnap(snap)["heartbeat"].(map[string]any)
	require.Equal(t, runnerID.String(), hb["runner_id"])
	require.Greater(t, hb["age_seconds"].(float64), float64(chatstate.HeartbeatStaleSeconds))
	require.True(t, hb["is_stale"].(bool))
}

// TestGetChatDebugSnapshot_Runtime_Unowned verifies the runtime section when
// the chat has no owner. worker_id_matches_local must be false and no runners.
func TestGetChatDebugSnapshot_Runtime_Unowned(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	client, _ := newChatClientWithAPI(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)

	chat := createDebugChat(ctx, t, client, firstUser.OrganizationID)

	snap := getDebugSnapshot(t, client, chat.ID)
	rt := rtSnap(snap)
	require.False(t, rt["worker_id_matches_local"].(bool))
	require.Empty(t, rt["runners"])
}

// TestGetChatDebugSnapshot_MultiReplica_ProxiesToOwner verifies that when the
// chat is owned by a different replica the proxy is invoked and its response
// is returned verbatim.
func TestGetChatDebugSnapshot_MultiReplica_ProxiesToOwner(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	mockRunnerID := uuid.New()
	mockWorkerID := uuid.New()
	mockKind := string(chatd.TaskKindGeneration)

	var capturedReplicaID uuid.UUID
	proxyCalled := make(chan struct{}, 1)
	client, _, api := newChatClientWithAPIAndDatabase(t,
		withChatWorkerDisabled,
		func(o *coderdtest.Options) {
			o.ChatDebugProxy = func(rw http.ResponseWriter, r *http.Request, replicaID uuid.UUID) {
				capturedReplicaID = replicaID
				select {
				case proxyCalled <- struct{}{}:
				default:
				}
				// Write a minimal snapshot response as if from the owning pod.
				rw.Header().Set("Content-Type", "application/json")
				rw.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(rw).Encode(map[string]any{
					"execution_state": "R0",
					"database":        map[string]any{},
					"runtime": map[string]any{
						"local_worker_id":         mockWorkerID.String(),
						"worker_id_matches_local": true,
						"runners": []any{
							map[string]any{
								"runner_id":        mockRunnerID.String(),
								"worker_id":        mockWorkerID.String(),
								"active_task_kind": mockKind,
							},
						},
					},
				})
			}
		},
	)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)

	chat := createDebugChat(ctx, t, client, firstUser.OrganizationID)

	// Assign a foreign worker_id so the proxy is invoked.
	foreignWorkerID := uuid.New()
	chatdCtx := dbauthz.AsChatd(ctx) //nolint:gocritic
	machine := chatstate.NewChatMachine(api.Database, api.Pubsub, chat.ID)
	require.NoError(t, machine.Update(chatdCtx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.Acquire(chatstate.AcquireInput{
			WorkerID: foreignWorkerID,
			RunnerID: uuid.New(),
		})
		return err
	}))

	snap := getDebugSnapshot(t, client, chat.ID)

	select {
	case <-proxyCalled:
	default:
		t.Fatal("proxy was not called")
	}
	require.Equal(t, foreignWorkerID, capturedReplicaID)

	rt := rtSnap(snap)
	runners := rt["runners"].([]any)
	require.Len(t, runners, 1)
	r0 := runners[0].(map[string]any)
	require.Equal(t, mockRunnerID.String(), r0["runner_id"])
	require.Equal(t, mockKind, r0["active_task_kind"])
}

// TestGetChatDebugSnapshot_MultiReplica_ProxyError verifies that when the proxy
// returns an error the client receives a non-200 response.
func TestGetChatDebugSnapshot_MultiReplica_ProxyError(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	client, _, api := newChatClientWithAPIAndDatabase(t,
		withChatWorkerDisabled,
		func(o *coderdtest.Options) {
			o.ChatDebugProxy = func(rw http.ResponseWriter, r *http.Request, _ uuid.UUID) {
				rw.WriteHeader(http.StatusBadGateway)
				_, _ = rw.Write([]byte(`{"message":"owning replica not found"}`))
			}
		},
	)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)

	chat := createDebugChat(ctx, t, client, firstUser.OrganizationID)

	// Assign a foreign worker_id to trigger the proxy.
	chatdCtx := dbauthz.AsChatd(ctx) //nolint:gocritic
	machine := chatstate.NewChatMachine(api.Database, api.Pubsub, chat.ID)
	require.NoError(t, machine.Update(chatdCtx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.Acquire(chatstate.AcquireInput{
			WorkerID: uuid.New(),
			RunnerID: uuid.New(),
		})
		return err
	}))

	resp, err := client.Request(ctx, http.MethodGet,
		"/api/experimental/chats/"+chat.ID.String()+"/debug/snapshot", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadGateway, resp.StatusCode)
}
