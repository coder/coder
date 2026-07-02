package coderd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/txtar"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// The types below mirror the (unexported) response type for
// GET /api/experimental/chats/{chat}/debug/snapshot, i.e. chatDebugSnapshot
// and friends in exp_chats.go. They are redeclared here because this file
// lives in the external coderd_test package and the production types are
// unexported; keep the JSON tags in sync with exp_chats.go.
type debugSnapshot struct {
	ExecutionState string                `json:"execution_state"`
	Database       debugSnapshotDatabase `json:"database"`
	Runtime        debugSnapshotRuntime  `json:"runtime"`
}

type debugSnapshotDatabase struct {
	Status                   string                    `json:"status"`
	Archived                 bool                      `json:"archived"`
	GenerationAttempt        int64                     `json:"generation_attempt"`
	SnapshotVersion          int64                     `json:"snapshot_version"`
	HistoryVersion           int64                     `json:"history_version"`
	WorkerID                 *uuid.UUID                `json:"worker_id"`
	RunnerID                 *uuid.UUID                `json:"runner_id"`
	LastError                json.RawMessage           `json:"last_error"`
	RetryState               json.RawMessage           `json:"retry_state"`
	RequiresActionDeadlineAt *time.Time                `json:"requires_action_deadline_at"`
	CreatedAt                time.Time                 `json:"created_at"`
	UpdatedAt                time.Time                 `json:"updated_at"`
	QueueDepth               int64                     `json:"queue_depth"`
	Heartbeat                *debugSnapshotHeartbeat   `json:"heartbeat"`
	MessageStats             debugSnapshotMessageStats `json:"message_stats"`
}

type debugSnapshotHeartbeat struct {
	RunnerID    uuid.UUID `json:"runner_id"`
	HeartbeatAt time.Time `json:"heartbeat_at"`
	AgeSeconds  float64   `json:"age_seconds"`
	IsStale     bool      `json:"is_stale"`
	Error       string    `json:"error,omitempty"`
}

type debugSnapshotMessageStats struct {
	Total   int64            `json:"total"`
	Deleted int64            `json:"deleted"`
	ByRole  map[string]int64 `json:"by_role"`
}

type debugSnapshotRuntime struct {
	LocalWorkerID        uuid.UUID                    `json:"local_worker_id"`
	WorkerIDMatchesLocal bool                         `json:"worker_id_matches_local"`
	Runners              []chatd.RunnerSnapshot       `json:"runners"`
	MessageBuffers       []debugSnapshotBufferEpisode `json:"message_buffers"`
}

type debugSnapshotBufferEpisode struct {
	HistoryVersion    int64 `json:"history_version"`
	GenerationAttempt int64 `json:"generation_attempt"`
	PartsBuffered     int   `json:"parts_buffered"`
	BytesBuffered     int64 `json:"bytes_buffered"`
	SubscriberCount   int   `json:"subscriber_count"`
	IsClosed          bool  `json:"is_closed"`
}

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

// getDebugSnapshot fetches and decodes the debug snapshot for a chat.
func getDebugSnapshot(t *testing.T, client *codersdk.ExperimentalClient, chatID uuid.UUID) debugSnapshot {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	resp, err := client.Request(ctx, http.MethodGet,
		"/api/experimental/chats/"+chatID.String()+"/debug/snapshot", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out debugSnapshot
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out
}

// assertDebugSnapshotGolden compares snap against the golden txtar fixture
// at testdata/chatdebugsnapshot/<name>.txtar, following the same pattern as
// the golden file tests in insights_test.go: non-deterministic fields
// (timestamps, the per-instance local worker ID) are reset to their zero
// value directly on the typed struct, and the comparison is done with
// cmp.Diff on the decoded struct rather than on raw JSON text. Any
// dynamically-generated UUIDs that appear elsewhere in the payload (e.g.
// runner/worker IDs) should be arranged by the caller to be deterministic
// fixture values instead of uuid.New(), matching the convention used for
// insights golden files.
//
// Callers must be tests whose name matches `make gen`'s `Test.*Golden$`
// filter (see coderd/.gen-golden in the Makefile); otherwise
// `make clean/golden-files` deletes the fixture and it never gets
// regenerated. Run `go test ./coderd -run "Test.*Golden$" -update`, or
// `make gen/golden-files`, to regenerate fixtures.
func assertDebugSnapshotGolden(t *testing.T, name string, snap debugSnapshot) {
	t.Helper()

	normalizeDebugSnapshot(&snap)

	goldenPath := filepath.Join("testdata", "chatdebugsnapshot", name+".json.golden")
	if *updateGoldenFiles {
		require.NoError(t, os.MkdirAll(filepath.Dir(goldenPath), 0o755), "want no error creating golden file directory")
		data, err := json.MarshalIndent(snap, "", "  ")
		require.NoError(t, err)
		data = append(data, '\n')
		ar := &txtar.Archive{Files: []txtar.File{{Name: "response.json", Data: data}}}
		require.NoError(t, os.WriteFile(goldenPath, txtar.Format(ar), 0o600), "want no error writing golden file")
		return
	}

	raw, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "open golden file, run \"go test ./coderd -run 'Test.*Golden$' -update\" and commit the changes")
	ar := txtar.Parse(raw)
	require.Len(t, ar.Files, 1, "golden file %s should contain exactly one file", goldenPath)

	var want debugSnapshot
	require.NoError(t, json.Unmarshal(ar.Files[0].Data, &want), "want no error decoding golden file")

	cmpOpts := []cmp.Option{
		// Ensure readable UUIDs in diff, matching insights_test.go.
		cmp.Transformer("UUID", func(id uuid.UUID) string { return id.String() }),
	}
	assert.Empty(t, cmp.Diff(want, snap, cmpOpts...),
		"golden file mismatch (-want +got): %s, run \"go test ./coderd -run 'Test.*Golden$' -update\" and commit the changes", goldenPath)
}

// normalizeDebugSnapshot zeroes fields that are never deterministic across
// test runs (wall-clock timestamps, durations derived from them, and the
// per-chatd.Server random worker ID) so golden comparisons are stable.
func normalizeDebugSnapshot(snap *debugSnapshot) {
	snap.Database.CreatedAt = time.Time{}
	snap.Database.UpdatedAt = time.Time{}
	snap.Database.RequiresActionDeadlineAt = nil
	if snap.Database.Heartbeat != nil {
		snap.Database.Heartbeat.HeartbeatAt = time.Time{}
		snap.Database.Heartbeat.AgeSeconds = 0
	}
	snap.Runtime.LocalWorkerID = uuid.Nil
}

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
		wantQueue  int64
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
			require.Equal(t, tc.wantState, snap.ExecutionState)
			require.Equal(t, tc.wantStatus, snap.Database.Status)
			require.Equal(t, tc.wantQueue, snap.Database.QueueDepth)
			if tc.name == "E0" {
				// last_error is a JSON object; verify it contains the expected message.
				var lastErr struct {
					Message string `json:"message"`
				}
				require.NoError(t, json.Unmarshal(snap.Database.LastError, &lastErr), "last_error should be a JSON object")
				require.Equal(t, "test error", lastErr.Message)
			}
		})
	}
}

// TestGetChatDebugSnapshot_MessageStats_Golden verifies the message_stats breakdown.
func TestGetChatDebugSnapshot_MessageStats_Golden(t *testing.T) {
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
	assertDebugSnapshotGolden(t, "message_stats", snap)
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
	require.Nil(t, snap.Database.Heartbeat)

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
	hb := snap.Database.Heartbeat
	require.NotNil(t, hb)
	require.Equal(t, runnerID, hb.RunnerID)
	require.GreaterOrEqual(t, hb.AgeSeconds, float64(0))
	require.False(t, hb.IsStale)

	// Advance the clock past the staleness threshold and verify is_stale flips.
	mClock.Advance((chatstate.HeartbeatStaleSeconds + 1) * time.Second)

	snap = getDebugSnapshot(t, client, chat.ID)
	hb = snap.Database.Heartbeat
	require.NotNil(t, hb)
	require.Equal(t, runnerID, hb.RunnerID)
	require.Greater(t, hb.AgeSeconds, float64(chatstate.HeartbeatStaleSeconds))
	require.True(t, hb.IsStale)
}

// TestGetChatDebugSnapshot_ChatDaemonDisabled verifies the endpoint returns
// 503 (instead of panicking on a nil chatDaemon) when the AI Gateway is
// disabled, matching every other chatDaemon-dependent handler in this file.
func TestGetChatDebugSnapshot_ChatDaemonDisabled(t *testing.T) {
	t.Parallel()

	values := coderdtest.DeploymentValues(t, func(v *codersdk.DeploymentValues) {
		v.AI.BridgeConfig.Enabled = false
	})
	opts := newChatTestOptions(t, values)
	client, closer, api := coderdtest.NewWithAPI(t, opts)
	defer closer.Close()
	firstUser := coderdtest.CreateFirstUser(t, client)

	// Insert a chat directly via the database, bypassing the create-chat
	// HTTP handler (which itself requires the chat daemon).
	provider := dbgen.ChatProvider(t, api.Database, database.ChatProvider{})
	modelConfig := dbgen.ChatModelConfig(t, api.Database, database.ChatModelConfig{
		AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true},
	})
	chat := dbgen.Chat(t, api.Database, database.Chat{
		OwnerID:           firstUser.UserID,
		OrganizationID:    firstUser.OrganizationID,
		LastModelConfigID: modelConfig.ID,
	})

	ctx := testutil.Context(t, testutil.WaitShort)
	resp, err := client.Request(ctx, http.MethodGet,
		"/api/experimental/chats/"+chat.ID.String()+"/debug/snapshot", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

// TestGetChatDebugSnapshot_Runtime_Unowned_Golden verifies the runtime section when
// the chat has no owner. worker_id_matches_local must be false and no runners.
func TestGetChatDebugSnapshot_Runtime_Unowned_Golden(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	client, _ := newChatClientWithAPI(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)

	chat := createDebugChat(ctx, t, client, firstUser.OrganizationID)

	snap := getDebugSnapshot(t, client, chat.ID)
	assertDebugSnapshotGolden(t, "runtime_unowned", snap)
}

// TestGetChatDebugSnapshot_MultiReplica_ProxiesToOwner_Golden verifies that when the
// chat is owned by a different replica the proxy is invoked and its response
// is returned verbatim.
func TestGetChatDebugSnapshot_MultiReplica_ProxiesToOwner_Golden(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	// Fixed, deterministic IDs (rather than uuid.New()) so the mocked
	// response can be compared directly against a golden fixture, matching
	// the fixture convention used elsewhere (e.g. coderd/insights_test.go).
	mockRunnerID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	mockWorkerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
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
				_ = json.NewEncoder(rw).Encode(debugSnapshot{
					ExecutionState: "R0",
					Runtime: debugSnapshotRuntime{
						LocalWorkerID:        mockWorkerID,
						WorkerIDMatchesLocal: true,
						Runners: []chatd.RunnerSnapshot{
							{
								RunnerID:       mockRunnerID,
								WorkerID:       mockWorkerID,
								ActiveTaskKind: (*chatd.TaskKind)(&mockKind),
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

	assertDebugSnapshotGolden(t, "multi_replica_proxies_to_owner", snap)
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
