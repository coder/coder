package database_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/coderd/x/chatd/agentselect"
	"github.com/coder/coder/v2/testutil"
)

const chatFileDeliveryLease = 5 * time.Minute

type chatFileDeliveryFixture struct {
	db    database.Store
	owner database.User
	org   database.Organization
	model database.ChatModelConfig
}

func newChatFileDeliveryFixture(t *testing.T) chatFileDeliveryFixture {
	t.Helper()
	sqlDB := testSQLDB(t)
	require.NoError(t, migrations.Up(sqlDB))
	db := database.New(sqlDB)
	return chatFileDeliveryFixture{
		db:    db,
		owner: dbgen.User(t, db, database.User{}),
		org:   dbgen.Organization(t, db, database.Organization{}),
		model: dbgen.ChatModelConfig(t, db, database.ChatModelConfig{}),
	}
}

// build inserts the next build of workspace. A zero workspace creates a
// new one.
func (f chatFileDeliveryFixture) build(t *testing.T, workspace database.WorkspaceTable, number int32, transition database.WorkspaceTransition) (database.WorkspaceTable, database.WorkspaceBuild) {
	t.Helper()
	if workspace.ID == uuid.Nil {
		workspace = database.WorkspaceTable{OwnerID: f.owner.ID, OrganizationID: f.org.ID}
	}
	resp := dbfake.WorkspaceBuild(t, f.db, workspace).
		Seed(database.WorkspaceBuild{BuildNumber: number, Transition: transition}).
		Do()
	return resp.Workspace, resp.Build
}

func (f chatFileDeliveryFixture) agent(ctx context.Context, t *testing.T, build database.WorkspaceBuild, seed database.WorkspaceAgent) database.WorkspaceAgent {
	t.Helper()
	resource := dbgen.WorkspaceResource(t, f.db, database.WorkspaceResource{JobID: build.JobID})
	agent := dbgen.WorkspaceAgent(t, f.db, database.WorkspaceAgent{
		ResourceID:   resource.ID,
		Name:         seed.Name,
		DisplayOrder: seed.DisplayOrder,
		ParentID:     seed.ParentID,
	})
	require.NoError(t, f.db.UpdateWorkspaceAgentLifecycleStateByID(ctx, database.UpdateWorkspaceAgentLifecycleStateByIDParams{
		ID:             agent.ID,
		LifecycleState: seed.LifecycleState,
	}))
	return agent
}

func (f chatFileDeliveryFixture) chat(t *testing.T, workspaceID, agentID uuid.UUID) database.Chat {
	t.Helper()
	return dbgen.Chat(t, f.db, database.Chat{
		OrganizationID:    f.org.ID,
		OwnerID:           f.owner.ID,
		LastModelConfigID: f.model.ID,
		WorkspaceID:       uuid.NullUUID{UUID: workspaceID, Valid: workspaceID != uuid.Nil},
		AgentID:           uuid.NullUUID{UUID: agentID, Valid: agentID != uuid.Nil},
	})
}

// readyChat returns a chat bound to a started workspace whose only agent
// finished startup.
func (f chatFileDeliveryFixture) readyChat(ctx context.Context, t *testing.T) database.Chat {
	t.Helper()
	workspace, build := f.build(t, database.WorkspaceTable{}, 1, database.WorkspaceTransitionStart)
	f.agent(ctx, t, build, database.WorkspaceAgent{Name: "dev", LifecycleState: database.WorkspaceAgentLifecycleStateReady})
	return f.chat(t, workspace.ID, uuid.Nil)
}

func (f chatFileDeliveryFixture) linkedFile(ctx context.Context, t *testing.T, chatID uuid.UUID, purpose database.ChatFilePurpose, data []byte) database.InsertChatFileRow {
	t.Helper()
	file, err := f.db.InsertChatFile(ctx, database.InsertChatFileParams{
		OwnerID:        f.owner.ID,
		OrganizationID: f.org.ID,
		Name:           "report.zip",
		Mimetype:       "application/zip",
		Data:           data,
		Purpose:        database.NullChatFilePurpose{ChatFilePurpose: purpose, Valid: purpose != ""},
	})
	require.NoError(t, err)
	rejected, err := f.db.LinkChatFiles(ctx, database.LinkChatFilesParams{
		ChatID:       chatID,
		FileIds:      []uuid.UUID{file.ID},
		MaxFileLinks: 10,
	})
	require.NoError(t, err)
	require.Zero(t, rejected)
	return file
}

func acquireChatFiles(ctx context.Context, t *testing.T, db database.Store, chatID uuid.NullUUID, now time.Time, limit int32) []database.AcquireChatWorkspaceFileDeliveriesRow {
	t.Helper()
	rows, err := db.AcquireChatWorkspaceFileDeliveries(ctx, database.AcquireChatWorkspaceFileDeliveriesParams{
		Now:             now,
		LeaseExpiresAt:  now.Add(chatFileDeliveryLease),
		ChatAgentSuffix: agentselect.Suffix,
		MaxAttempts:     database.ChatWorkspaceFileMaxDeliveryAttempts,
		ChatID:          chatID,
		LimitVal:        limit,
	})
	require.NoError(t, err)
	return rows
}

func acquireChatFilesFor(ctx context.Context, t *testing.T, db database.Store, chatID uuid.UUID, now time.Time) []database.AcquireChatWorkspaceFileDeliveriesRow {
	t.Helper()
	return acquireChatFiles(ctx, t, db, uuid.NullUUID{UUID: chatID, Valid: true}, now, 10)
}

func TestAcquireChatWorkspaceFileDeliveries(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	f := newChatFileDeliveryFixture(t)

	t.Run("StartupPredicate", func(t *testing.T) {
		t.Parallel()

		started := func(state database.WorkspaceAgentLifecycleState) func(context.Context, *testing.T) database.Chat {
			return func(ctx context.Context, t *testing.T) database.Chat {
				workspace, build := f.build(t, database.WorkspaceTable{}, 1, database.WorkspaceTransitionStart)
				f.agent(ctx, t, build, database.WorkspaceAgent{Name: "dev", LifecycleState: state})
				return f.chat(t, workspace.ID, uuid.Nil)
			}
		}
		for _, tc := range []struct {
			name    string
			chat    func(context.Context, *testing.T) database.Chat
			claimed bool
		}{
			{name: "Ready", chat: started(database.WorkspaceAgentLifecycleStateReady), claimed: true},
			{name: "StartError", chat: started(database.WorkspaceAgentLifecycleStateStartError), claimed: true},
			{name: "StartTimeout", chat: started(database.WorkspaceAgentLifecycleStateStartTimeout), claimed: true},
			{name: "Starting", chat: started(database.WorkspaceAgentLifecycleStateStarting)},
			{name: "Created", chat: started(database.WorkspaceAgentLifecycleStateCreated)},
			{name: "NoWorkspace", chat: func(_ context.Context, t *testing.T) database.Chat {
				return f.chat(t, uuid.Nil, uuid.Nil)
			}},
			{name: "StoppedAfterReadyStart", chat: func(ctx context.Context, t *testing.T) database.Chat {
				workspace, build := f.build(t, database.WorkspaceTable{}, 1, database.WorkspaceTransitionStart)
				f.agent(ctx, t, build, database.WorkspaceAgent{Name: "dev", LifecycleState: database.WorkspaceAgentLifecycleStateReady})
				f.build(t, workspace, 2, database.WorkspaceTransitionStop)
				return f.chat(t, workspace.ID, uuid.Nil)
			}},
			{name: "RestartingAfterReadyStart", chat: func(ctx context.Context, t *testing.T) database.Chat {
				workspace, build := f.build(t, database.WorkspaceTable{}, 1, database.WorkspaceTransitionStart)
				f.agent(ctx, t, build, database.WorkspaceAgent{Name: "dev", LifecycleState: database.WorkspaceAgentLifecycleStateReady})
				_, build = f.build(t, workspace, 2, database.WorkspaceTransitionStart)
				f.agent(ctx, t, build, database.WorkspaceAgent{Name: "dev", LifecycleState: database.WorkspaceAgentLifecycleStateStarting})
				return f.chat(t, workspace.ID, uuid.Nil)
			}},
			{name: "ArchivedChat", chat: func(ctx context.Context, t *testing.T) database.Chat {
				chat := f.readyChat(ctx, t)
				_, err := f.db.ArchiveChatByID(ctx, chat.ID)
				require.NoError(t, err)
				return chat
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)
				chat := tc.chat(ctx, t)
				file := f.linkedFile(ctx, t, chat.ID, database.ChatFilePurposeWorkspace, []byte("payload"))

				rows := acquireChatFilesFor(ctx, t, f.db, chat.ID, dbtime.Now())
				if !tc.claimed {
					require.Empty(t, rows)
					return
				}
				require.Len(t, rows, 1)
				require.Equal(t, file.ID, rows[0].ID)
				require.Equal(t, chat.ID, rows[0].ChatID)
				require.Equal(t, chat.WorkspaceID, rows[0].WorkspaceID)
				require.Equal(t, int32(1), rows[0].WorkspaceDeliveryAttempts)
				require.Equal(t, int64(len("payload")), rows[0].SizeBytes)
			})
		}
	})

	// The checked agent must be the one delivery would pick, so a ready
	// agent elsewhere in the workspace cannot release a file early.
	t.Run("AgentSelection", func(t *testing.T) {
		t.Parallel()

		ready := database.WorkspaceAgentLifecycleStateReady
		starting := database.WorkspaceAgentLifecycleStateStarting
		for _, tc := range []struct {
			name    string
			chat    func(context.Context, *testing.T, database.WorkspaceTable, database.WorkspaceBuild) database.Chat
			claimed bool
		}{
			{name: "ChatSuffixAgentStarting", chat: func(ctx context.Context, t *testing.T, ws database.WorkspaceTable, build database.WorkspaceBuild) database.Chat {
				f.agent(ctx, t, build, database.WorkspaceAgent{Name: "main", DisplayOrder: 1, LifecycleState: ready})
				f.agent(ctx, t, build, database.WorkspaceAgent{Name: "work" + agentselect.Suffix, DisplayOrder: 2, LifecycleState: starting})
				return f.chat(t, ws.ID, uuid.Nil)
			}},
			{name: "ChatSuffixAgentReady", chat: func(ctx context.Context, t *testing.T, ws database.WorkspaceTable, build database.WorkspaceBuild) database.Chat {
				f.agent(ctx, t, build, database.WorkspaceAgent{Name: "main", DisplayOrder: 1, LifecycleState: starting})
				f.agent(ctx, t, build, database.WorkspaceAgent{Name: "work" + agentselect.Suffix, DisplayOrder: 2, LifecycleState: ready})
				return f.chat(t, ws.ID, uuid.Nil)
			}, claimed: true},
			{name: "FirstByDisplayOrderStarting", chat: func(ctx context.Context, t *testing.T, ws database.WorkspaceTable, build database.WorkspaceBuild) database.Chat {
				f.agent(ctx, t, build, database.WorkspaceAgent{Name: "b", DisplayOrder: 1, LifecycleState: starting})
				f.agent(ctx, t, build, database.WorkspaceAgent{Name: "a", DisplayOrder: 2, LifecycleState: ready})
				return f.chat(t, ws.ID, uuid.Nil)
			}},
			{name: "FirstByNameReady", chat: func(ctx context.Context, t *testing.T, ws database.WorkspaceTable, build database.WorkspaceBuild) database.Chat {
				f.agent(ctx, t, build, database.WorkspaceAgent{Name: "b", DisplayOrder: 1, LifecycleState: starting})
				f.agent(ctx, t, build, database.WorkspaceAgent{Name: "a", DisplayOrder: 1, LifecycleState: ready})
				return f.chat(t, ws.ID, uuid.Nil)
			}, claimed: true},
			{name: "BoundChildAgentReady", chat: func(ctx context.Context, t *testing.T, ws database.WorkspaceTable, build database.WorkspaceBuild) database.Chat {
				root := f.agent(ctx, t, build, database.WorkspaceAgent{Name: "dev", DisplayOrder: 1, LifecycleState: starting})
				child := f.agent(ctx, t, build, database.WorkspaceAgent{
					Name:           "z-child",
					DisplayOrder:   1,
					ParentID:       uuid.NullUUID{UUID: root.ID, Valid: true},
					LifecycleState: ready,
				})
				return f.chat(t, ws.ID, child.ID)
			}, claimed: true},
			{name: "UnboundChildAgentReady", chat: func(ctx context.Context, t *testing.T, ws database.WorkspaceTable, build database.WorkspaceBuild) database.Chat {
				root := f.agent(ctx, t, build, database.WorkspaceAgent{Name: "dev", DisplayOrder: 1, LifecycleState: starting})
				f.agent(ctx, t, build, database.WorkspaceAgent{
					Name:           "child" + agentselect.Suffix,
					DisplayOrder:   1,
					ParentID:       uuid.NullUUID{UUID: root.ID, Valid: true},
					LifecycleState: ready,
				})
				return f.chat(t, ws.ID, uuid.Nil)
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)
				workspace, build := f.build(t, database.WorkspaceTable{}, 1, database.WorkspaceTransitionStart)
				chat := tc.chat(ctx, t, workspace, build)
				f.linkedFile(ctx, t, chat.ID, database.ChatFilePurposeWorkspace, []byte("payload"))

				rows := acquireChatFilesFor(ctx, t, f.db, chat.ID, dbtime.Now())
				if tc.claimed {
					require.Len(t, rows, 1)
				} else {
					require.Empty(t, rows)
				}
			})
		}
	})

	t.Run("FileState", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		now := dbtime.Now()
		chat := f.readyChat(ctx, t)

		f.linkedFile(ctx, t, chat.ID, "", []byte("prompt"))
		delivered := f.linkedFile(ctx, t, chat.ID, database.ChatFilePurposeWorkspace, []byte("delivered"))
		parked := f.linkedFile(ctx, t, chat.ID, database.ChatFilePurposeWorkspace, []byte("parked"))
		require.Len(t, acquireChatFilesFor(ctx, t, f.db, chat.ID, now), 2)
		completed, err := f.db.CompleteChatWorkspaceFileDelivery(ctx, database.CompleteChatWorkspaceFileDeliveryParams{
			ID:                   delivered.ID,
			DeliveredAt:          now,
			DeliveredPath:        "~/report.zip",
			DeliveredWorkspaceID: chat.WorkspaceID.UUID,
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, completed)
		failed, err := f.db.FailChatWorkspaceFileDelivery(ctx, database.FailChatWorkspaceFileDeliveryParams{
			ID:            parked.ID,
			Error:         "agent rejected the file",
			NextAttemptAt: now,
			Mode:          database.ChatWorkspaceFileDeliveryFailurePark,
			MaxAttempts:   database.ChatWorkspaceFileMaxDeliveryAttempts,
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, failed)

		require.Empty(t, acquireChatFilesFor(ctx, t, f.db, chat.ID, now.Add(time.Hour)))
	})
}

// TestAcquireChatWorkspaceFileDeliveriesConcurrentClaimers uses its own
// database because it claims without a chat filter.
func TestAcquireChatWorkspaceFileDeliveriesConcurrentClaimers(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	ctx := testutil.Context(t, testutil.WaitLong)
	f := newChatFileDeliveryFixture(t)
	now := dbtime.Now()
	chat := f.readyChat(ctx, t)
	first := f.linkedFile(ctx, t, chat.ID, database.ChatFilePurposeWorkspace, []byte("first"))
	second := f.linkedFile(ctx, t, chat.ID, database.ChatFilePurposeWorkspace, []byte("second"))

	err := f.db.InTx(func(tx database.Store) error {
		held := acquireChatFiles(ctx, t, tx, uuid.NullUUID{}, now, 1)
		require.Len(t, held, 1)
		// The first claim has not committed, so its row stays locked and
		// a concurrent claimer must skip it rather than wait or take it.
		other := acquireChatFiles(ctx, t, f.db, uuid.NullUUID{}, now, 2) // intxcheck:ignore // a second connection models a concurrent replica
		require.Len(t, other, 1)
		require.ElementsMatch(t, []uuid.UUID{first.ID, second.ID}, []uuid.UUID{held[0].ID, other[0].ID})
		return nil
	}, nil)
	require.NoError(t, err)

	require.Empty(t, acquireChatFiles(ctx, t, f.db, uuid.NullUUID{}, now.Add(chatFileDeliveryLease-time.Second), 2))

	reclaimed := acquireChatFiles(ctx, t, f.db, uuid.NullUUID{}, now.Add(chatFileDeliveryLease), 2)
	require.Len(t, reclaimed, 2)
	for _, row := range reclaimed {
		require.Equal(t, int32(2), row.WorkspaceDeliveryAttempts)
	}
}

func TestChatWorkspaceFileDeliveryOutcomes(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	f := newChatFileDeliveryFixture(t)

	t.Run("Complete", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name string
			data []byte
		}{
			{name: "WithContent", data: []byte("payload")},
			{name: "Empty", data: []byte{}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)
				now := dbtime.Now()
				chat := f.readyChat(ctx, t)
				file := f.linkedFile(ctx, t, chat.ID, database.ChatFilePurposeWorkspace, tc.data)
				require.Len(t, acquireChatFilesFor(ctx, t, f.db, chat.ID, now), 1)

				params := database.CompleteChatWorkspaceFileDeliveryParams{
					ID:                   file.ID,
					DeliveredAt:          now,
					DeliveredPath:        "~/.coder/chats/" + chat.ID.String() + "/files/" + file.ID.String() + "/report.zip",
					DeliveredWorkspaceID: chat.WorkspaceID.UUID,
				}
				completed, err := f.db.CompleteChatWorkspaceFileDelivery(ctx, params)
				require.NoError(t, err)
				require.EqualValues(t, 1, completed)

				stored, err := f.db.GetChatFileByID(ctx, file.ID)
				require.NoError(t, err)
				require.Empty(t, stored.Data)
				require.Equal(t, "report.zip", stored.Name)
				require.Equal(t, "application/zip", stored.Mimetype)

				byID, err := f.db.GetChatFilesMetadataByIDs(ctx, []uuid.UUID{file.ID})
				require.NoError(t, err)
				require.Len(t, byID, 1)
				require.Equal(t, database.ChatFilePurposeWorkspace, byID[0].Purpose)
				require.EqualValues(t, len(tc.data), byID[0].SizeBytes)
				require.WithinDuration(t, now, byID[0].WorkspaceDeliveredAt.Time, time.Millisecond)
				require.Equal(t, params.DeliveredPath, byID[0].WorkspaceDeliveredPath.String)
				require.Equal(t, uuid.NullUUID{UUID: params.DeliveredWorkspaceID, Valid: true}, byID[0].WorkspaceDeliveredWorkspaceID)

				byChat, err := f.db.GetChatFileMetadataByChatID(ctx, chat.ID)
				require.NoError(t, err)
				require.Len(t, byChat, 1)
				require.Equal(t, params.DeliveredPath, byChat[0].WorkspaceDeliveredPath.String)
				require.EqualValues(t, len(tc.data), byChat[0].SizeBytes)
				require.False(t, byChat[0].WorkspaceDeliveryNextAttemptAt.Valid)
				require.False(t, byChat[0].WorkspaceDeliveryError.Valid)

				again, err := f.db.CompleteChatWorkspaceFileDelivery(ctx, database.CompleteChatWorkspaceFileDeliveryParams{
					ID:                   file.ID,
					DeliveredAt:          now.Add(time.Minute),
					DeliveredPath:        "~/elsewhere",
					DeliveredWorkspaceID: uuid.New(),
				})
				require.NoError(t, err)
				require.Zero(t, again)
				byID, err = f.db.GetChatFilesMetadataByIDs(ctx, []uuid.UUID{file.ID})
				require.NoError(t, err)
				require.Equal(t, params.DeliveredPath, byID[0].WorkspaceDeliveredPath.String)
			})
		}
	})

	t.Run("FailAndRetry", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		now := dbtime.Now()
		chat := f.readyChat(ctx, t)
		file := f.linkedFile(ctx, t, chat.ID, database.ChatFilePurposeWorkspace, []byte("payload"))

		fail := func(mode string, next time.Time) {
			t.Helper()
			failed, err := f.db.FailChatWorkspaceFileDelivery(ctx, database.FailChatWorkspaceFileDeliveryParams{
				ID:            file.ID,
				Error:         mode + " failure",
				NextAttemptAt: next,
				Mode:          mode,
				MaxAttempts:   database.ChatWorkspaceFileMaxDeliveryAttempts,
			})
			require.NoError(t, err)
			require.EqualValues(t, 1, failed)
		}
		metadata := func() database.GetChatFileMetadataByChatIDRow {
			t.Helper()
			rows, err := f.db.GetChatFileMetadataByChatID(ctx, chat.ID)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			return rows[0]
		}

		require.Len(t, acquireChatFilesFor(ctx, t, f.db, chat.ID, now), 1)
		fail(database.ChatWorkspaceFileDeliveryFailureRetry, now.Add(time.Minute))
		got := metadata()
		require.Equal(t, int32(1), got.WorkspaceDeliveryAttempts)
		require.Equal(t, "retry failure", got.WorkspaceDeliveryError.String)
		require.Empty(t, acquireChatFilesFor(ctx, t, f.db, chat.ID, now.Add(time.Minute-time.Second)))

		now = now.Add(time.Minute)
		claimed := acquireChatFilesFor(ctx, t, f.db, chat.ID, now)
		require.Len(t, claimed, 1)
		require.Equal(t, int32(2), claimed[0].WorkspaceDeliveryAttempts)
		require.False(t, metadata().WorkspaceDeliveryError.Valid)
		fail(database.ChatWorkspaceFileDeliveryFailureRefund, now.Add(time.Minute))
		require.Equal(t, int32(1), metadata().WorkspaceDeliveryAttempts)

		now = now.Add(time.Minute)
		require.Len(t, acquireChatFilesFor(ctx, t, f.db, chat.ID, now), 1)
		fail(database.ChatWorkspaceFileDeliveryFailurePark, now)
		require.Equal(t, database.ChatWorkspaceFileMaxDeliveryAttempts, metadata().WorkspaceDeliveryAttempts)
		require.Empty(t, acquireChatFilesFor(ctx, t, f.db, chat.ID, now.Add(time.Hour)))

		_, err := f.db.FailChatWorkspaceFileDelivery(ctx, database.FailChatWorkspaceFileDeliveryParams{
			ID:            file.ID,
			Error:         "unknown",
			NextAttemptAt: now,
			Mode:          "unknown",
			MaxAttempts:   database.ChatWorkspaceFileMaxDeliveryAttempts,
		})
		require.Error(t, err)

		f.linkedFile(ctx, t, chat.ID, database.ChatFilePurposeWorkspace, []byte("in flight"))
		require.Len(t, acquireChatFilesFor(ctx, t, f.db, chat.ID, now), 1)

		retried, err := f.db.RetryChatWorkspaceFileDeliveries(ctx, chat.ID)
		require.NoError(t, err)
		require.EqualValues(t, 1, retried)

		claimed = acquireChatFilesFor(ctx, t, f.db, chat.ID, now)
		require.Len(t, claimed, 1)
		require.Equal(t, file.ID, claimed[0].ID)
		require.Equal(t, int32(1), claimed[0].WorkspaceDeliveryAttempts)
	})

	t.Run("RetryRecoversUnrecordedLastAttempt", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		chat := f.readyChat(ctx, t)
		file := f.linkedFile(ctx, t, chat.ID, database.ChatFilePurposeWorkspace, []byte("payload"))

		// Every attempt's replica stops before recording an outcome, so
		// each lease expires with no error and the last one uses up the
		// attempts.
		claimedAt := dbtime.Now().Add(-time.Hour)
		for range database.ChatWorkspaceFileMaxDeliveryAttempts {
			require.Len(t, acquireChatFilesFor(ctx, t, f.db, chat.ID, claimedAt), 1)
			claimedAt = claimedAt.Add(chatFileDeliveryLease)
		}
		require.Empty(t, acquireChatFilesFor(ctx, t, f.db, chat.ID, dbtime.Now()))

		retried, err := f.db.RetryChatWorkspaceFileDeliveries(ctx, chat.ID)
		require.NoError(t, err)
		require.EqualValues(t, 1, retried)
		claimed := acquireChatFilesFor(ctx, t, f.db, chat.ID, dbtime.Now())
		require.Len(t, claimed, 1)
		require.Equal(t, file.ID, claimed[0].ID)
		require.Equal(t, int32(1), claimed[0].WorkspaceDeliveryAttempts)
	})

	t.Run("PromptFileDefault", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		chat := f.readyChat(ctx, t)
		file := f.linkedFile(ctx, t, chat.ID, "", []byte("prompt"))

		rows, err := f.db.GetChatFilesMetadataByIDs(ctx, []uuid.UUID{file.ID})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, database.ChatFilePurposePrompt, file.Purpose)
		require.Equal(t, database.ChatFilePurposePrompt, rows[0].Purpose)
		require.Equal(t, int64(len("prompt")), rows[0].SizeBytes)
		require.Equal(t, sql.NullTime{}, rows[0].WorkspaceDeliveredAt)
	})
}
