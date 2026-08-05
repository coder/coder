package chatd

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

func TestPartitionAmbiguousToolCallsGatesOnBuiltins(t *testing.T) {
	t.Parallel()

	fetch := fetchToolStub()
	ambiguous := fantasy.ToolCallContent{
		ToolCallID: "call_ambiguous",
		ToolName:   "fetch",
		Input:      `{"URL":"https://example.test","url":"https://other.test"}`,
	}
	clean := fantasy.ToolCallContent{
		ToolCallID: "call_clean",
		ToolName:   "fetch",
		Input:      `{"url":"https://example.test"}`,
	}

	t.Run("builtin", func(t *testing.T) {
		t.Parallel()

		prepared := generationPrepared{
			Tools:            []fantasy.AgentTool{fetch},
			BuiltinToolNames: map[string]bool{"fetch": true},
		}
		allowed, rejected := partitionAmbiguousToolCalls(prepared, []fantasy.ToolCallContent{ambiguous, clean})
		require.Len(t, rejected, 1)
		require.Equal(t, "call_ambiguous", rejected[0].ToolCallID)
		require.Len(t, allowed, 1)
		require.Equal(t, "call_clean", allowed[0].ToolCallID)
	})

	t.Run("non-builtin", func(t *testing.T) {
		t.Parallel()

		prepared := generationPrepared{Tools: []fantasy.AgentTool{fetch}}
		allowed, rejected := partitionAmbiguousToolCalls(prepared, []fantasy.ToolCallContent{ambiguous, clean})
		require.Empty(t, rejected)
		require.Len(t, allowed, 2)
	})

	// coderd executes provider tools with a local runner itself, so their
	// input is guarded like a builtin even though the provider defines
	// the schema server-side.
	t.Run("locally run provider tool", func(t *testing.T) {
		t.Parallel()

		prepared := generationPrepared{
			ProviderTools: []chatloop.ProviderTool{{
				Runner: chattool.NewComputerUseTool(
					codersdk.ChatComputerUseProviderAnthropic,
					1024, 768,
					nil, nil,
					quartz.NewMock(t),
					slog.Make(),
				),
			}},
		}
		smuggled := fantasy.ToolCallContent{
			ToolCallID: "call_computer_smuggled",
			ToolName:   "computer",
			Input:      `{"action":"screenshot","ACTION":"key","text":"ctrl+alt+delete"}`,
		}
		nested := fantasy.ToolCallContent{
			ToolCallID: "call_computer_nested",
			ToolName:   "computer",
			Input:      `{"call_id":"c","actions":[{"type":"click","TYPE":"keypress","keys":["ctrl","alt","delete"]}]}`,
		}
		cleanComputer := fantasy.ToolCallContent{
			ToolCallID: "call_computer_clean",
			ToolName:   "computer",
			Input:      `{"action":"screenshot"}`,
		}
		allowed, rejected := partitionAmbiguousToolCalls(prepared, []fantasy.ToolCallContent{smuggled, nested, cleanComputer})
		require.Len(t, rejected, 2)
		require.Equal(t, "call_computer_smuggled", rejected[0].ToolCallID)
		require.Equal(t, "call_computer_nested", rejected[1].ToolCallID)
		require.Len(t, allowed, 1)
		require.Equal(t, "call_computer_clean", allowed[0].ToolCallID)
	})

	// A tokenizer failure mid-walk must reject the call: json.Unmarshal
	// tolerates values (such as huge exponents) inside ignored fields
	// that abort a Token walk, so acceptance would skip later keys.
	t.Run("poisoned value fails closed", func(t *testing.T) {
		t.Parallel()

		prepared := generationPrepared{
			Tools:            []fantasy.AgentTool{fetch},
			BuiltinToolNames: map[string]bool{"fetch": true},
		}
		poisoned := fantasy.ToolCallContent{
			ToolCallID: "call_poisoned",
			ToolName:   "fetch",
			Input:      `{"_x":1e999,"URL":"https://attacker.test"}`,
		}
		_, rejected := partitionAmbiguousToolCalls(prepared, []fantasy.ToolCallContent{poisoned})
		require.Len(t, rejected, 1)
	})

	// Execution resolves a deprecated name to its canonical tool, so
	// validation has to resolve it too.
	t.Run("deprecated alias", func(t *testing.T) {
		t.Parallel()

		var alias, canonical string
		for a, name := range subagentToolNameAliases {
			alias, canonical = a, name
			break
		}
		require.NotEmpty(t, canonical)

		type input struct {
			ChatID string `json:"chat_id"`
		}
		tool := fantasy.NewAgentTool(canonical, "",
			func(context.Context, input, fantasy.ToolCall) (fantasy.ToolResponse, error) {
				return fantasy.ToolResponse{}, nil
			})
		aliased := fantasy.ToolCallContent{
			ToolCallID: "call_aliased",
			ToolName:   alias,
			Input:      `{"chat_id":"a","CHAT_ID":"b"}`,
		}

		prepared := generationPrepared{
			Tools:            []fantasy.AgentTool{tool},
			BuiltinToolNames: map[string]bool{canonical: true},
		}
		_, rejected := partitionAmbiguousToolCalls(prepared, []fantasy.ToolCallContent{aliased})
		require.Len(t, rejected, 1)
	})
}

func TestValidateOverriddenToolInputs(t *testing.T) {
	t.Parallel()

	prepared := generationPrepared{
		Tools:            []fantasy.AgentTool{fetchToolStub()},
		BuiltinToolNames: map[string]bool{"fetch": true},
	}
	overridden := chathooks.PreToolUseExecutionResult{
		Allowed: []fantasy.ToolCallContent{{
			ToolCallID: "call_overridden",
			ToolName:   "fetch",
			Input:      `{"URL":"https://other.test"}`,
		}},
		Overrides: map[string]json.RawMessage{
			"call_overridden": json.RawMessage(`{"URL":"https://other.test"}`),
		},
	}
	require.ErrorContains(t, validateOverriddenToolInputs(prepared, overridden),
		`hook input override for tool fetch: input key "URL" differs from schema property "url" only by case`)

	// The same input is left alone when no consumer replaced it, because
	// the model-authored batch is checked before the dispatch instead.
	untouched := chathooks.PreToolUseExecutionResult{Allowed: overridden.Allowed}
	require.NoError(t, validateOverriddenToolInputs(prepared, untouched))
}

// TestBuiltinToolSchemasDescribeTheirInputs guards the validator's reach: it
// cannot detect a case-variant key for a builtin that declares no properties.
func TestBuiltinToolSchemasDescribeTheirInputs(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	provider := dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{
		Type: database.AIProviderTypeOpenai,
	}, "test-key")
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:        "gpt-4o-mini",
		AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true},
	}, func(p *database.InsertChatModelConfigParams) {
		p.Enabled = true
	})
	created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: modelConfig.ID,
		Title:             "builtin tool schemas",
		ClientType:        database.ChatClientTypeApi,
		InitialMessages: []chatstate.Message{{
			Role:           database.ChatMessageRoleUser,
			Content:        mustMarshalText(t, "hello"),
			Visibility:     database.ChatMessageVisibilityBoth,
			ModelConfigID:  uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
			CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
			ContentVersion: chatprompt.CurrentContentVersion,
		}},
	})
	require.NoError(t, err)

	server := newInternalTestServer(
		t,
		db,
		ps,
		chatprovider.ProviderAPIKeys{},
		withInternalTestServerTransportFactory(&aibridgeTestFactory{}),
	)
	prepared, err := server.prepareGeneration(ctx, generationPrepareInput{
		Chat:     created.Chat,
		Messages: created.InitialMessages,
	})
	require.NoError(t, err)
	t.Cleanup(prepared.Cleanup)

	// These take an empty struct, so they carry no keys to validate.
	noInput := map[string]bool{
		"process_list":         true,
		"stop_workspace":       true,
		"list_subagent_models": true,
	}
	var unvalidated []string
	require.NotEmpty(t, prepared.BuiltinToolNames)
	for _, tool := range prepared.Tools {
		info := tool.Info()
		if !prepared.BuiltinToolNames[info.Name] || len(info.Parameters) > 0 || noInput[info.Name] {
			continue
		}
		unvalidated = append(unvalidated, info.Name)
	}
	require.Empty(t, unvalidated,
		"these builtin tools declare no schema properties, so their input is not validated")
}

func fetchToolStub() fantasy.AgentTool {
	type input struct {
		URL string `json:"url"`
	}
	return fantasy.NewAgentTool("fetch", "",
		func(context.Context, input, fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.ToolResponse{}, nil
		})
}
