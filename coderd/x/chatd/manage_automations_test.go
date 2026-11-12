package chatd_test

import (
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	experimentrules "github.com/coder/coder/v2/coderd/experiments"
	"github.com/coder/coder/v2/coderd/experiments/experimentstest"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatautomation"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// automationsEvaluator returns an evaluator that has the experiment on, off,
// or unable to read its rules (which also means off).
func automationsEvaluator(t *testing.T, mode string) *experimentrules.Evaluator {
	t.Helper()
	static := codersdk.Experiments{}
	store := experimentstest.Store{}
	switch mode {
	case "on":
		static = codersdk.Experiments{codersdk.ExperimentChatAutomations}
	case "error":
		static = codersdk.Experiments{codersdk.ExperimentChatAutomations}
		store.RulesErr = xerrors.New("rules are unavailable")
	}
	evaluator, err := experimentrules.New(slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), store, static)
	require.NoError(t, err)
	return evaluator
}

// TestManageAutomationsAvailable checks each condition of the RFC sentence:
// the tool is available only in non-archived root chats outside plan and
// explore modes, with the switch on and the experiment on for the owner.
func TestManageAutomationsAvailable(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		evaluator string // "on", "off", "error" or "" for none
		chat      func(*database.Chat)
		want      bool
	}{
		"Available":      {evaluator: "on", want: true},
		"SwitchOff":      {evaluator: "on", chat: func(c *database.Chat) { c.ManageAutomationsEnabled = false }},
		"ExperimentOff":  {evaluator: "off"},
		"EvaluatorError": {evaluator: "error"},
		"NoEvaluator":    {},
		"SubAgent":       {evaluator: "on", chat: func(c *database.Chat) { c.ParentChatID = uuid.NullUUID{UUID: uuid.New(), Valid: true} }},
		"Archived":       {evaluator: "on", chat: func(c *database.Chat) { c.Archived = true }},
		"PlanMode": {evaluator: "on", chat: func(c *database.Chat) {
			c.PlanMode = database.NullChatPlanMode{ChatPlanMode: database.ChatPlanModePlan, Valid: true}
		}},
		"ExploreMode": {evaluator: "on", chat: func(c *database.Chat) {
			c.Mode = database.NullChatMode{ChatMode: database.ChatModeExplore, Valid: true}
		}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db, ps := dbtestutil.NewDB(t)
			server := newTestServer(t, db, ps, uuid.New(), func(cfg *chatd.Config) {
				if tc.evaluator != "" {
					cfg.ExperimentEvaluator = automationsEvaluator(t, tc.evaluator)
				}
			})
			chat := database.Chat{OwnerID: uuid.New(), ManageAutomationsEnabled: true}
			if tc.chat != nil {
				tc.chat(&chat)
			}
			require.Equal(t, tc.want, server.ManageAutomationsAvailable(testutil.Context(t, testutil.WaitLong), chat))
		})
	}
}

// TestManageAutomationsDefault creates root and sub-agent chats and reads the
// starting value of the switch.
func TestManageAutomationsDefault(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		evaluator string
		child     bool
		want      bool
	}{
		"RootExperimentOn":  {evaluator: "on", want: true},
		"RootExperimentOff": {evaluator: "off"},
		"RootNoEvaluator":   {},
		"ChildExperimentOn": {evaluator: "on", child: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			db, ps := dbtestutil.NewDB(t)
			user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", "http://127.0.0.1:0")
			server := newTestServer(t, db, ps, uuid.New(), func(cfg *chatd.Config) {
				if tc.evaluator != "" {
					cfg.ExperimentEvaluator = automationsEvaluator(t, tc.evaluator)
				}
			})
			opts := chatd.CreateOptions{OrganizationID: org.ID, OwnerID: user.ID, Title: "default", ModelConfigID: model.ID}
			if tc.child {
				root, err := server.CreateChat(ctx, opts)
				require.NoError(t, err)
				opts.ParentChatID = uuid.NullUUID{UUID: root.ID, Valid: true}
				opts.RootChatID = uuid.NullUUID{UUID: root.ID, Valid: true}
			}

			chat, err := server.CreateChat(ctx, opts)

			require.NoError(t, err)
			require.Equal(t, tc.want, chat.ManageAutomationsEnabled)
			stored, err := db.GetChatByID(ctx, chat.ID)
			require.NoError(t, err)
			require.Equal(t, tc.want, stored.ManageAutomationsEnabled)
		})
	}
}

// TestManageAutomationsTurn drives full turns on a fake provider. With the
// experiment on, the request lists the tool, and a list call and a get call
// reach the chat history without the webhook secret, also in the debug
// records. With the experiment off, the request has no such tool although the
// chat's switch is on.
func TestManageAutomationsTurn(t *testing.T) {
	t.Parallel()

	for name, experiment := range map[string]string{"ExperimentOn": "on", "ExperimentOff": "off"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
			secretCanary, promptCanary := "canary-secret-"+uuid.NewString(), "canary-prompt-"+uuid.NewString()
			var automationID atomic.Value
			var (
				mu    sync.Mutex
				tools [][]string
			)
			var calls atomic.Int32
			openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
				if !req.Stream {
					return chattest.OpenAINonStreamingResponse("title")
				}
				names := []string{}
				for _, tool := range req.Tools {
					names = append(names, tool.Function.Name)
				}
				mu.Lock()
				tools = append(tools, names)
				mu.Unlock()
				switch calls.Add(1) {
				case 1:
					if experiment == "on" {
						return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk(chatautomation.ToolName, `{"action":"list"}`))
					}
				case 2:
					return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk(chatautomation.ToolName,
						fmt.Sprintf(`{"action":"get","id":%q}`, automationID.Load())))
				}
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
			})
			user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
			automation := dbgen.ChatAutomation(t, db, database.ChatAutomation{
				OrganizationID: org.ID,
				OwnerID:        user.ID,
				Prompt:         promptCanary,
				WebhookSecret:  sql.NullString{String: secretCanary, Valid: true},
			})
			automationID.Store(automation.ID.String())
			server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
				cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
				cfg.AlwaysEnableDebugLogs = true
				cfg.ExperimentEvaluator = automationsEvaluator(t, experiment)
			})
			// Created idle, so that the switch of the experiment-off chat can be
			// turned on before its first turn.
			chat, err := server.CreateChat(ctx, chatd.CreateOptions{OrganizationID: org.ID, OwnerID: user.ID, Title: "automations", ModelConfigID: model.ID})
			require.NoError(t, err)
			require.Equal(t, experiment == "on", chat.ManageAutomationsEnabled)
			_, err = db.UpdateChatManageAutomationsEnabledByID(ctx, database.UpdateChatManageAutomationsEnabledByIDParams{
				ID: chat.ID, ManageAutomationsEnabled: true,
			})
			require.NoError(t, err)

			_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
				ChatID:        chat.ID,
				CreatedBy:     user.ID,
				ModelConfigID: model.ID,
				Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("what automations do I have?")},
			})
			require.NoError(t, err)
			waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

			mu.Lock()
			first := tools[0]
			mu.Unlock()
			if experiment == "off" {
				require.NotContains(t, first, chatautomation.ToolName)
				require.Equal(t, int32(1), calls.Load())
				return
			}
			require.Contains(t, first, chatautomation.ToolName)
			parts := chatToolParts(ctx, t, db, chat.ID)
			var results []codersdk.ChatMessagePart
			for _, part := range parts {
				if part.Type == codersdk.ChatMessagePartTypeToolResult && part.ToolName == chatautomation.ToolName {
					results = append(results, part)
				}
			}
			require.Len(t, results, 2)
			for _, result := range results {
				require.False(t, result.IsError, string(result.Result))
				require.Contains(t, string(result.Result), automation.ID.String())
				require.Contains(t, string(result.Result), promptCanary)
			}

			// The secret is in no history row and no debug record. The prompt
			// shows that the same rows hold the results.
			count := func(table, column, needle string) int {
				var n int
				require.NoError(t, sqlDB.QueryRowContext(ctx,
					fmt.Sprintf(`SELECT count(*) FROM %s WHERE chat_id = $1 AND %s::text LIKE '%%' || $2 || '%%'`, table, column),
					chat.ID, needle).Scan(&n))
				return n
			}
			require.Zero(t, count("chat_messages", "content", secretCanary))
			require.NotZero(t, count("chat_messages", "content", promptCanary))
			require.Zero(t, count("chat_debug_steps", "normalized_request", secretCanary))
			require.Zero(t, count("chat_debug_steps", "normalized_response", secretCanary))
			require.NotZero(t, count("chat_debug_steps", "normalized_request", promptCanary), "the second step holds the first tool result")
		})
	}
}
