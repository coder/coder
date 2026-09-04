package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatacp"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

var testHarnesses = chatacp.Harnesses()

// testHarness is any harness row for helpers whose behavior does not
// depend on the runtime; harness-specific copy is checked per harness.
var testHarness = testHarnesses[0]

func TestExternalHarness(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		runtime      database.ChatRuntime
		experiments  codersdk.Experiments
		wantExternal bool
		wantMessage  string
	}{
		{name: "CoderNeedsNoExperiment", runtime: database.ChatRuntimeCoder},
		{name: "ExternalWithExperiment", runtime: database.ChatRuntimeCodex, experiments: codersdk.Experiments{codersdk.ExperimentAgentsRuntimeConfig}, wantExternal: true},
		{name: "ExternalWithoutExperiment", runtime: database.ChatRuntimeClaudeCode, wantMessage: "This chat uses an external runtime, but the agents-runtime-config experiment is disabled."},
		{name: "UnknownRuntime", runtime: database.ChatRuntime("unknown"), experiments: codersdk.Experiments{codersdk.ExperimentAgentsRuntimeConfig}, wantMessage: "This chat uses an unsupported runtime."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := &Server{experiments: tc.experiments}
			harness, isExternal, err := server.externalHarness(tc.runtime)
			require.Equal(t, tc.wantExternal, isExternal)
			if tc.wantMessage != "" {
				classified := chaterror.Classify(err)
				require.Equal(t, codersdk.ChatErrorKindConfig, classified.Kind)
				require.Equal(t, tc.wantMessage, classified.Message)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantExternal, harness.Runtime == codersdk.ChatRuntime(tc.runtime))
		})
	}
}

func TestWaitForACPAdapter(t *testing.T) {
	t.Parallel()

	adapterMissing := xerrors.New("exit status 1")
	// Every failed non-final probe arms exactly one poll timer, so the
	// clock choreography follows from wantProbes alone.
	tests := []struct {
		name string
		// succeedOnProbe is the probe that finds the adapter; 0 never does.
		succeedOnProbe int
		// settledAfterProbes marks the scripts settled once that many
		// probes have run; -1 keeps them running.
		settledAfterProbes int
		wantProbes         int
		wantErr            bool
	}{
		{name: "RetriesWhileScriptsRun", succeedOnProbe: 3, settledAfterProbes: -1, wantProbes: 3},
		{name: "SettledScriptsFailImmediately", settledAfterProbes: 0, wantProbes: 1, wantErr: true},
		{name: "DeadlineBoundsUnsettledScripts", settledAfterProbes: -1, wantProbes: int(acpWorkspaceReadyTimeout/acpWorkspacePollInterval) + 1, wantErr: true},
		{name: "ScriptsSettlingAfterFailedProbeReprobes", succeedOnProbe: 2, settledAfterProbes: 1, wantProbes: 2},
	}
	for _, harness := range testHarnesses {
		t.Run(string(harness.Runtime), func(t *testing.T) {
			t.Parallel()
			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					ctx := testutil.Context(t, testutil.WaitShort)
					clock := quartz.NewMock(t)
					deadline := clock.Now("chatworker", "chatacp-readiness").Add(acpWorkspaceReadyTimeout)
					trap := clock.Trap().NewTimer("chatworker", "chatacp-preflight")
					defer trap.Close()

					probes := 0
					probe := func(context.Context) error {
						probes++
						if probes == tc.succeedOnProbe {
							return nil
						}
						return adapterMissing
					}
					settled := func(context.Context) bool {
						return tc.settledAfterProbes >= 0 && probes >= tc.settledAfterProbes
					}

					done := make(chan error, 1)
					go func() {
						done <- waitForACPAdapter(ctx, clock, harness, deadline, probe, settled)
					}()
					for range tc.wantProbes - 1 {
						trap.MustWait(ctx).MustRelease(ctx)
						clock.Advance(acpWorkspacePollInterval).MustWait(ctx)
					}
					err := testutil.RequireReceive(ctx, t, done)
					require.Equal(t, tc.wantProbes, probes)
					if !tc.wantErr {
						require.NoError(t, err)
						return
					}
					classified := chaterror.Classify(err)
					require.Equal(t, codersdk.ChatErrorKindConfig, classified.Kind)
					require.Contains(t, classified.Message, "the "+harness.DisplayName+" adapter ("+harness.Command+")")
				})
			}
		})
	}
}

func TestACPNewSessionCwd(t *testing.T) {
	t.Parallel()

	const expanded = "/home/coder/project"
	tests := []struct {
		name  string
		agent database.WorkspaceAgent
		// expandOnReload is the reload that reports the expansion; 0
		// never does.
		expandOnReload int
		wantReloads    int
		wantCwd        string
	}{
		{name: "ExpansionKnown", agent: database.WorkspaceAgent{Directory: "~/project", ExpandedDirectory: expanded}, wantCwd: expanded},
		{name: "NoDirectoryConfigured", agent: database.WorkspaceAgent{}},
		{name: "WaitsForExpansion", agent: database.WorkspaceAgent{Directory: "~/project"}, expandOnReload: 2, wantReloads: 2, wantCwd: expanded},
		{name: "DeadlineFallsBack", agent: database.WorkspaceAgent{Directory: "~/project"}, wantReloads: int(acpWorkspaceReadyTimeout / acpWorkspacePollInterval)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			clock := quartz.NewMock(t)
			deadline := clock.Now("chatworker", "chatacp-readiness").Add(acpWorkspaceReadyTimeout)
			trap := clock.Trap().NewTimer("chatworker", "chatacp-cwd")
			defer trap.Close()

			reloads := 0
			reload := func(context.Context) (database.WorkspaceAgent, error) {
				reloads++
				agent := tc.agent
				if reloads == tc.expandOnReload {
					agent.ExpandedDirectory = expanded
				}
				return agent, nil
			}

			type result struct {
				cwd string
				err error
			}
			done := make(chan result, 1)
			go func() {
				cwd, err := acpNewSessionCwd(ctx, clock, tc.agent, deadline, reload)
				done <- result{cwd: cwd, err: err}
			}()
			for range tc.wantReloads {
				trap.MustWait(ctx).MustRelease(ctx)
				clock.Advance(acpWorkspacePollInterval).MustWait(ctx)
			}
			got := testutil.RequireReceive(ctx, t, done)
			require.NoError(t, got.err)
			require.Equal(t, tc.wantCwd, got.cwd)
			require.Equal(t, tc.wantReloads, reloads)
		})
	}
}

// TestPersistACPRuntimeStateSkipsResetSession verifies that a turn only
// records its session while the stored state is still the one it
// started from, so a message edit that reset the session mid-turn wins.
func TestPersistACPRuntimeStateSkipsResetSession(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Runtime:        database.ChatRuntime(testHarness.Runtime),
	})
	starter := &taskStarter{opts: chatWorkerOptions{
		Store:  db,
		Logger: testutil.NewFakeSink(t).Logger(),
	}}
	storedState := func() chatacp.RuntimeState {
		got, err := db.GetChatByID(ctx, chat.ID)
		require.NoError(t, err)
		return chatacp.ParseRuntimeState(got.RuntimeState.RawMessage)
	}
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	// The first turn starts from an empty state and records its session.
	first := chatacp.RuntimeState{SessionID: "session-1", Cwd: "/home/coder/project", UpdatedAt: now}
	starter.persistACPRuntimeState(ctx, chat.ID, chatacp.RuntimeState{}, first)
	require.Equal(t, first, storedState())

	// An edit resets the session while the second turn is in flight;
	// that turn's late write must not resurrect the discarded session.
	reset := chatacp.RuntimeState{UpdatedAt: now.Add(time.Minute)}
	encoded, err := json.Marshal(reset)
	require.NoError(t, err)
	rows, err := db.UpdateChatRuntimeState(ctx, database.UpdateChatRuntimeStateParams{
		ID:                chat.ID,
		RuntimeState:      pqtype.NullRawMessage{RawMessage: encoded, Valid: true},
		ExpectedUpdatedAt: sql.NullString{String: first.UpdatedAtText(), Valid: true},
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)
	stale := chatacp.RuntimeState{SessionID: "session-1", Cwd: "/home/coder/project", UpdatedAt: now.Add(2 * time.Minute)}
	starter.persistACPRuntimeState(ctx, chat.ID, first, stale)
	require.Equal(t, reset, storedState())

	// The turn that started after the reset records its new session.
	fresh := chatacp.RuntimeState{SessionID: "session-2", Cwd: "/home/coder/project", UpdatedAt: now.Add(3 * time.Minute)}
	starter.persistACPRuntimeState(ctx, chat.ID, reset, fresh)
	require.Equal(t, fresh, storedState())
}

func acpMessage(t *testing.T, id int64, role database.ChatMessageRole, part codersdk.ChatMessagePart) database.ChatMessage {
	t.Helper()
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{part})
	require.NoError(t, err)
	return database.ChatMessage{
		ID:             id,
		Role:           role,
		Content:        content,
		ContentVersion: chatprompt.CurrentContentVersion,
	}
}

func acpTextMessage(t *testing.T, id int64, role database.ChatMessageRole, text string) database.ChatMessage {
	t.Helper()
	return acpMessage(t, id, role, codersdk.ChatMessageText(text))
}

func withModelConfig(msg database.ChatMessage, id uuid.UUID) database.ChatMessage {
	msg.ModelConfigID = uuid.NullUUID{UUID: id, Valid: true}
	return msg
}

func TestACPTurnFromHistory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slog.Logger{}

	selected, superseded := uuid.New(), uuid.New()
	hookNotice := acpMessage(t, 2, database.ChatMessageRoleSystem, codersdk.ChatMessagePart{
		Type: codersdk.ChatMessagePartTypeHookNotice,
		Text: "hook ran",
	})
	filePart := acpMessage(t, 1, database.ChatMessageRoleUser, codersdk.ChatMessagePart{
		Type: codersdk.ChatMessagePartTypeFile, FileName: "img.png", MediaType: "image/png",
	})

	tests := []struct {
		name    string
		history []database.ChatMessage
		want    acpTurn
		// wantErr rows run per harness, since the classified message
		// names the runtime.
		wantErr string
	}{
		{
			name: "EmptyHistory",
		},
		{
			name:    "SingleUserMessage",
			history: []database.ChatMessage{acpTextMessage(t, 1, database.ChatMessageRoleUser, "hello")},
			want:    acpTurn{generate: true, prompt: "hello"},
		},
		{
			name: "TrailingUserRunJoined",
			history: []database.ChatMessage{
				acpTextMessage(t, 1, database.ChatMessageRoleUser, "first"),
				acpTextMessage(t, 2, database.ChatMessageRoleAssistant, "reply"),
				withModelConfig(acpTextMessage(t, 3, database.ChatMessageRoleUser, "second"), superseded),
				withModelConfig(acpTextMessage(t, 4, database.ChatMessageRoleUser, "third"), selected),
			},
			want: acpTurn{
				generate:      true,
				prompt:        "second\n\nthird",
				reseed:        []chatacp.ReseedTurn{{Role: "User", Text: "first"}, {Role: "Assistant", Text: "reply"}},
				modelConfigID: selected,
			},
		},
		{
			name: "HistoryEndsWithAssistant",
			history: []database.ChatMessage{
				acpTextMessage(t, 1, database.ChatMessageRoleUser, "hello"),
				acpTextMessage(t, 2, database.ChatMessageRoleAssistant, "done"),
			},
		},
		{
			name: "TrailingHookNoticeStillGenerates",
			history: []database.ChatMessage{
				withModelConfig(acpTextMessage(t, 1, database.ChatMessageRoleUser, "hello"), selected),
				withModelConfig(hookNotice, superseded),
			},
			want: acpTurn{generate: true, prompt: "hello", modelConfigID: selected},
		},
		{
			name:    "NonTextUserMessageErrors",
			history: []database.ChatMessage{filePart},
			wantErr: "no text content",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.wantErr == "" {
				turn, err := acpTurnFromHistory(ctx, logger, testHarness, tc.history)
				require.NoError(t, err)
				require.Equal(t, tc.want, turn)
				return
			}
			for _, harness := range testHarnesses {
				_, err := acpTurnFromHistory(ctx, logger, harness, tc.history)
				require.ErrorContains(t, err, tc.wantErr)
				require.Equal(t, harness.DisplayName+" chats currently support text messages only.", chaterror.Classify(err).Message)
			}
		})
	}
}
