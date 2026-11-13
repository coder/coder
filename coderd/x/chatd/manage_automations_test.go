package chatd_test

import (
	"context"
	"fmt"
	"slices"
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

// automationsEvaluator returns an evaluator that has the experiment on or off.
func automationsEvaluator(t *testing.T, mode string) *experimentrules.Evaluator {
	t.Helper()
	static := codersdk.Experiments{}
	if mode == "on" {
		static = codersdk.Experiments{codersdk.ExperimentChatAutomations}
	}
	evaluator, err := experimentrules.New(slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), experimentstest.Store{}, static)
	require.NoError(t, err)
	return evaluator
}

// flipStore is a rules store that fails to read once broken is set. The
// evaluator then fails closed and decides that the experiment is off.
type flipStore struct {
	experimentstest.Store
	broken atomic.Bool
}

func (s *flipStore) Rules(ctx context.Context) (map[codersdk.Experiment]experimentrules.StoredRule, error) {
	if s.broken.Load() {
		return nil, xerrors.New("rules are unavailable")
	}
	return s.Store.Rules(ctx)
}

// TestManageAutomationsAvailable checks each condition of the RFC sentence:
// the tool is available only in non-archived root chats outside plan and
// explore modes, with the switch on and the experiment on for the owner.
func TestManageAutomationsAvailable(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		evaluator string // "on" or "off"
		chat      func(*database.Chat)
		want      bool
	}{
		"Available":     {evaluator: "on", want: true},
		"SwitchOff":     {evaluator: "on", chat: func(c *database.Chat) { c.ManageAutomationsEnabled = false }},
		"ExperimentOff": {evaluator: "off"},
		"SubAgent":      {evaluator: "on", chat: func(c *database.Chat) { c.ParentChatID = uuid.NullUUID{UUID: uuid.New(), Valid: true} }},
		"Archived":      {evaluator: "on", chat: func(c *database.Chat) { c.Archived = true }},
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
				cfg.ExperimentEvaluator = automationsEvaluator(t, tc.evaluator)
			})
			chat := database.Chat{OwnerID: uuid.New(), ManageAutomationsEnabled: true}
			if tc.chat != nil {
				tc.chat(&chat)
			}
			require.Equal(t, tc.want, chatd.ManageAutomationsAvailable(server, testutil.Context(t, testutil.WaitLong), chat))
		})
	}
}

// TestManageAutomationsTurn drives full turns on a fake provider. With the
// experiment on, the request lists the tool, and a list call and a get call
// reach the chat history. The experiment is read again for each step: when it
// turns off while the second step runs, the third request has no tool, and the
// call that the second step returned is refused.
func TestManageAutomationsTurn(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		offInSecondStep bool
	}{
		"ExperimentOn":              {},
		"ExperimentOffInSecondStep": {offInSecondStep: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			db, ps := dbtestutil.NewDB(t)
			promptCanary := "canary-prompt-" + uuid.NewString()
			var automationID atomic.Value
			var (
				mu    sync.Mutex
				tools [][]string
			)
			var calls atomic.Int32
			evaluator := automationsEvaluator(t, "on")
			var rules flipStore
			if tc.offInSecondStep {
				var err error
				evaluator, err = experimentrules.New(slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), &rules,
					codersdk.Experiments{codersdk.ExperimentChatAutomations})
				require.NoError(t, err)
			}
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
					return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk(chatautomation.ToolName, `{"action":"list"}`))
				case 2:
					if tc.offInSecondStep {
						rules.broken.Store(true)
					}
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
			})
			automationID.Store(automation.ID.String())
			server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
				cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
				cfg.ExperimentEvaluator = evaluator
			})
			chat, err := server.CreateChat(ctx, chatd.CreateOptions{OrganizationID: org.ID, OwnerID: user.ID, Title: "automations", ModelConfigID: model.ID})
			require.NoError(t, err)
			require.True(t, chat.ManageAutomationsEnabled)

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
			require.Contains(t, first, chatautomation.ToolName)
			parts := chatToolParts(ctx, t, db, chat.ID)
			var results []codersdk.ChatMessagePart
			for _, part := range parts {
				if part.Type == codersdk.ChatMessagePartTypeToolResult && part.ToolName == chatautomation.ToolName {
					results = append(results, part)
				}
			}
			require.Len(t, results, 2)
			if tc.offInSecondStep {
				mu.Lock()
				steps := slices.Clone(tools)
				mu.Unlock()
				require.Len(t, steps, 3)
				require.Contains(t, steps[1], chatautomation.ToolName)
				require.NotContains(t, steps[2], chatautomation.ToolName)
				require.False(t, results[0].IsError, string(results[0].Result))
				require.True(t, results[1].IsError)
				require.Contains(t, string(results[1].Result), "Tool not active in this turn")
				return
			}
			for _, result := range results {
				require.False(t, result.IsError, string(result.Result))
				require.Contains(t, string(result.Result), automation.ID.String())
				require.Contains(t, string(result.Result), promptCanary)
			}
		})
	}
}
