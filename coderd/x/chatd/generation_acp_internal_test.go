package chatd

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatacp"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// testHarness is any harness row for helpers whose behavior does not
// depend on the runtime; harness-specific copy is checked per harness.
var testHarness = mustHarness(codersdk.ChatRuntimeClaudeCode)

func mustHarness(runtime codersdk.ChatRuntime) chatacp.Harness {
	harness, ok := chatacp.HarnessFor(runtime)
	if !ok {
		panic("no harness for runtime " + string(runtime))
	}
	return harness
}

var testHarnesses = []chatacp.Harness{
	mustHarness(codersdk.ChatRuntimeClaudeCode),
	mustHarness(codersdk.ChatRuntimeCodex),
}

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

	t.Run("RetriesWhileScriptsRun", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		deadline := clock.Now("chatworker", "chatacp-readiness").Add(acpWorkspaceReadyTimeout)
		trap := clock.Trap().NewTimer("chatworker", "chatacp-preflight")
		defer trap.Close()

		probes := 0
		probe := func(context.Context) error {
			probes++
			if probes >= 3 {
				return nil
			}
			return adapterMissing
		}

		done := make(chan error, 1)
		go func() {
			done <- waitForACPAdapter(ctx, clock, testHarness, deadline, probe,
				func(context.Context) bool { return false })
		}()

		for range 2 {
			call := trap.MustWait(ctx)
			call.MustRelease(ctx)
			clock.Advance(acpWorkspacePollInterval).MustWait(ctx)
		}
		require.NoError(t, testutil.RequireReceive(ctx, t, done))
		require.Equal(t, 3, probes)
	})

	t.Run("SettledScriptsFailImmediately", func(t *testing.T) {
		t.Parallel()
		for _, harness := range testHarnesses {
			t.Run(string(harness.Runtime), func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitShort)
				clock := quartz.NewMock(t)
				deadline := clock.Now("chatworker", "chatacp-readiness").Add(acpWorkspaceReadyTimeout)

				probes := 0
				err := waitForACPAdapter(ctx, clock, harness, deadline,
					func(context.Context) error { probes++; return adapterMissing },
					func(context.Context) bool { return true })
				require.Error(t, err)
				require.Equal(t, 1, probes)
				classified := chaterror.Classify(err)
				require.Equal(t, codersdk.ChatErrorKindConfig, classified.Kind)
				require.Contains(t, classified.Message, "the "+harness.DisplayName+" adapter ("+harness.Command+")")
			})
		}
	})

	t.Run("DeadlineBoundsUnsettledScripts", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		deadline := clock.Now("chatworker", "chatacp-readiness").Add(acpWorkspaceReadyTimeout)
		trap := clock.Trap().NewTimer("chatworker", "chatacp-preflight")
		defer trap.Close()

		done := make(chan error, 1)
		go func() {
			done <- waitForACPAdapter(ctx, clock, testHarness, deadline,
				func(context.Context) error { return adapterMissing },
				func(context.Context) bool { return false })
		}()

		elapsed := time.Duration(0)
		for elapsed < acpWorkspaceReadyTimeout {
			call := trap.MustWait(ctx)
			call.MustRelease(ctx)
			clock.Advance(acpWorkspacePollInterval).MustWait(ctx)
			elapsed += acpWorkspacePollInterval
		}
		err := testutil.RequireReceive(ctx, t, done)
		require.Error(t, err)
		classified := chaterror.Classify(err)
		require.Equal(t, codersdk.ChatErrorKindConfig, classified.Kind)
	})

	t.Run("ScriptsSettlingAfterFailedProbeReprobes", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		deadline := clock.Now("chatworker", "chatacp-readiness").Add(acpWorkspaceReadyTimeout)
		trap := clock.Trap().NewTimer("chatworker", "chatacp-preflight")
		defer trap.Close()

		probes := 0
		probe := func(context.Context) error {
			probes++
			if probes >= 2 {
				return nil
			}
			return adapterMissing
		}
		settledCalls := 0
		settled := func(context.Context) bool {
			settledCalls++
			return settledCalls > 1
		}

		done := make(chan error, 1)
		go func() {
			done <- waitForACPAdapter(ctx, clock, testHarness, deadline, probe, settled)
		}()

		call := trap.MustWait(ctx)
		call.MustRelease(ctx)
		clock.Advance(acpWorkspacePollInterval).MustWait(ctx)
		require.NoError(t, testutil.RequireReceive(ctx, t, done))
		require.Equal(t, 2, probes)
	})
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
