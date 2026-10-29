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
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

func TestPartitionToolCallsValidatesSchema(t *testing.T) {
	t.Parallel()

	type target struct {
		Path string `json:"path"`
	}
	type input struct {
		Target     target                       `json:"target"`
		Count      int                          `json:"count,omitempty"`
		Parameters map[string]string            `json:"parameters,omitempty"`
		Targets    []map[string]target          `json:"targets,omitempty"`
		Nested     map[string]map[string]string `json:"nested,omitempty"`
	}
	for _, tt := range []struct {
		input   string
		wantErr string
	}{
		{input: `{}`, wantErr: `missing properties: ["target"]`},
		{input: `{"target":{}}`, wantErr: `missing properties: ["path"]`},
		{input: `{"target":{"path":1}}`, wantErr: `want "string"`},
		{input: `{"target":{"path":"a"},"count":1.5}`, wantErr: `want "integer"`},
		{input: `null`, wantErr: `want "object"`},
		{input: `[]`, wantErr: `want "object"`},
		{input: `{"target":{"path":"a"},"parameters":{"cpu":42}}`, wantErr: `want "string"`},
		{input: `{"target":{"path":"a"},"targets":[{"first":{}}]}`, wantErr: `missing properties: ["path"]`},
		{input: `{"target":{"path":"a"},"nested":{"first":{"cpu":42}}}`, wantErr: `want "string"`},
		{input: `{"target":{"path":"a"},"extra":1}`},
		{input: `{"target":{"path":"a"},"count":1,"parameters":{"cpu":"2","CPU":"4"},"targets":[{"first":{"path":"b"}}],"nested":{"first":{"cpu":"2"}}}`},
	} {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			tool := fantasy.NewAgentTool("test", "", func(context.Context, input, fantasy.ToolCall) (fantasy.ToolResponse, error) {
				return fantasy.ToolResponse{}, nil
			})
			prepared := turnEnvironment{Tools: []fantasy.AgentTool{tool}, BuiltinToolNames: map[string]bool{"test": true}}
			call := fantasy.ToolCallContent{ToolName: "test", Input: tt.input}
			allowed, _, rejected := partitionInvalidToolCalls(prepared, []fantasy.ToolCallContent{call})
			if tt.wantErr != "" {
				require.Empty(t, allowed)
				require.Len(t, rejected, 1)
				require.ErrorContains(t, rejected[0].Result.(fantasy.ToolResultOutputContentError).Error, tt.wantErr)
			} else {
				require.Empty(t, rejected)
				require.Equal(t, []fantasy.ToolCallContent{call}, allowed)
			}
		})
	}
}

func TestPartitionToolCallsValidatesACPSpawnSchema(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		input   string
		wantErr string
	}{
		{input: `{"harness_id":"claude-code","prompt":"Report which MCP servers are available to you.","working_directory":"/home/coder/coder"}`, wantErr: `missing properties: ["agent"]`},
		{input: `{"prompt":"task","working_directory":"/work","agent":{}}`, wantErr: `missing properties: ["harness_id"]`},
		{input: `{"prompt":"task","working_directory":"/work","agent":{"harness_id":"unknown"}}`, wantErr: "enum"},
		{input: `{"prompt":"task","working_directory":"/work","agent":{"harness_id":"claude-code","config":{"mode":"invalid"}}}`, wantErr: "enum"},
		{input: `{"prompt":"task","working_directory":"/work","agent":{"harness_id":"other","config":{"mode":"code"}}}`, wantErr: "unexpected additional properties"},
		{input: `{"prompt":"task","working_directory":"/work","agent":{"harness_id":"claude-code","extra":true}}`, wantErr: "unexpected additional properties"},
		{input: `{"prompt":"task","working_directory":"/work","agent":{"harness_id":"claude-code","config":{"mode":"code"}}}`},
		{input: `{"prompt":"task","working_directory":"/work","agent":{"harness_id":"other"}}`},
	} {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			tools := newACPTools(acpToolset{harnesses: []workspacesdk.ACPHarness{
				{Slug: "claude-code", ConfigOptions: []workspacesdk.ACPConfigOption{
					{ID: "mode", Values: []workspacesdk.ACPConfigValue{{ID: "code"}}},
				}},
				{Slug: "other"},
			}})
			prepared := turnEnvironment{Tools: tools, BuiltinToolNames: map[string]bool{"acp_spawn_agent": true}}
			call := fantasy.ToolCallContent{ToolName: "acp_spawn_agent", Input: tt.input}
			allowed, _, rejected := partitionInvalidToolCalls(prepared, []fantasy.ToolCallContent{call})
			if tt.wantErr != "" {
				require.Empty(t, allowed)
				require.Len(t, rejected, 1)
				result := rejected[0].Result.(fantasy.ToolResultOutputContentError)
				require.ErrorContains(t, result.Error, tt.wantErr)
				return
			}
			require.Empty(t, rejected)
			require.Len(t, allowed, 1)
		})
	}
}

func TestPartitionInvalidToolCallsGatesOnBuiltins(t *testing.T) {
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

		prepared := turnEnvironment{
			Tools:            []fantasy.AgentTool{fetch},
			BuiltinToolNames: map[string]bool{"fetch": true},
		}
		allowed, allowedIndexes, rejected := partitionInvalidToolCalls(prepared, []fantasy.ToolCallContent{ambiguous, clean})
		require.Len(t, rejected, 1)
		require.Equal(t, "call_ambiguous", rejected[0].ToolCallID)
		require.Len(t, allowed, 1)
		require.Equal(t, "call_clean", allowed[0].ToolCallID)
		require.Equal(t, []int{1}, allowedIndexes, "allowed indexes point at positions in the input slice")
	})

	t.Run("non-builtin", func(t *testing.T) {
		t.Parallel()

		prepared := turnEnvironment{Tools: []fantasy.AgentTool{fetch}}
		missingRequired := fantasy.ToolCallContent{ToolName: "fetch", Input: `{}`}
		allowed, allowedIndexes, rejected := partitionInvalidToolCalls(prepared, []fantasy.ToolCallContent{ambiguous, clean, missingRequired})
		require.Empty(t, rejected)
		require.Len(t, allowed, 3)
		require.Equal(t, []int{0, 1, 2}, allowedIndexes)
		require.NoError(t, validateOverriddenToolInputs(prepared, chathooks.PreToolUseExecutionResult{
			Allowed:   []fantasy.ToolCallContent{missingRequired},
			Overrides: map[string]json.RawMessage{missingRequired.ToolCallID: json.RawMessage(missingRequired.Input)},
		}))
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

		prepared := turnEnvironment{
			Tools:            []fantasy.AgentTool{tool},
			BuiltinToolNames: map[string]bool{canonical: true},
		}
		missingRequired := fantasy.ToolCallContent{ToolName: alias, Input: `{}`}
		_, _, rejected := partitionInvalidToolCalls(prepared, []fantasy.ToolCallContent{aliased, missingRequired})
		require.Len(t, rejected, 2)
	})
}

func TestValidateOverriddenToolInputs(t *testing.T) {
	t.Parallel()

	prepared := turnEnvironment{
		Tools:            []fantasy.AgentTool{fetchToolStub()},
		BuiltinToolNames: map[string]bool{"fetch": true},
	}
	for _, tt := range []struct {
		input   string
		wantErr string
	}{
		{input: `{"URL":"https://other.test"}`, wantErr: `input key "URL" differs from schema property "url" only by case`},
		{input: `{}`, wantErr: `missing properties: ["url"]`},
		{input: `{"url":1}`, wantErr: `want "string"`},
		{input: `null`, wantErr: `want "object"`},
		{input: `{"url":"https://other.test"}`},
	} {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			overridden := chathooks.PreToolUseExecutionResult{
				Allowed: []fantasy.ToolCallContent{{ToolCallID: "call_overridden", ToolName: "fetch", Input: tt.input}},
				Overrides: map[string]json.RawMessage{
					"call_overridden": json.RawMessage(tt.input),
				},
			}
			err := validateOverriddenToolInputs(prepared, overridden)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "hook input override for tool fetch:")
				require.ErrorContains(t, err, tt.wantErr)
			}

			// Model-authored inputs are checked before hook dispatch instead.
			untouched := chathooks.PreToolUseExecutionResult{Allowed: overridden.Allowed}
			require.NoError(t, validateOverriddenToolInputs(prepared, untouched))
		})
	}
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
	prepared, err := server.buildTurnEnvironment(ctx, generationPrepareInput{
		Chat:            created.Chat,
		Messages:        created.InitialMessages,
		TurnExperiments: &turnExperimentDecisions{},
	})
	require.NoError(t, err)
	t.Cleanup(prepared.Close)

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
