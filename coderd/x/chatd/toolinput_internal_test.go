package chatd

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
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
		allowed, allowedIndexes, rejected := partitionAmbiguousToolCalls(prepared, []fantasy.ToolCallContent{ambiguous, clean})
		require.Len(t, rejected, 1)
		require.Equal(t, "call_ambiguous", rejected[0].ToolCallID)
		require.Len(t, allowed, 1)
		require.Equal(t, "call_clean", allowed[0].ToolCallID)
		require.Equal(t, []int{1}, allowedIndexes, "allowed indexes point at positions in the input slice")
	})

	t.Run("non-builtin", func(t *testing.T) {
		t.Parallel()

		prepared := generationPrepared{Tools: []fantasy.AgentTool{fetch}}
		allowed, allowedIndexes, rejected := partitionAmbiguousToolCalls(prepared, []fantasy.ToolCallContent{ambiguous, clean})
		require.Empty(t, rejected)
		require.Len(t, allowed, 2)
		require.Equal(t, []int{0, 1}, allowedIndexes)
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
		_, _, rejected := partitionAmbiguousToolCalls(prepared, []fantasy.ToolCallContent{aliased})
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
		Model:          "gpt-4o-mini",
		AIProviderID:   uuid.NullUUID{UUID: provider.ID, Valid: true},
		OrganizationID: org.ID,
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
		"get_goal":             true,
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
