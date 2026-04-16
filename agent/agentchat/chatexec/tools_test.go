package chatexec_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentchat/chatexec"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

func TestExecutor_BuildsControlPlaneBuiltinTools(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	runtimeContext := defaultRuntimeContext()
	runtimeContext.ChatID = chatID
	runtimeContext.BuiltinTools = []agentsdk.ChatRunnerToolDefinition{
		{Name: "list_templates"},
		{Name: "read_file"},
		{Name: "read_template"},
		{Name: "create_workspace"},
	}

	client := &mockChatRunnerClient{runtimeContextResp: runtimeContext}
	executor := newTestExecutor(
		t,
		client,
		nil,
		func(_ context.Context, opts chatloop.RunOptions) error {
			require.Equal(t, []string{
				"read_file",
				"list_templates",
				"read_template",
			}, agentToolNames(opts.Tools))
			return nil
		},
		func(context.Context) (workspacesdk.AgentConn, error) {
			return nil, xerrors.New("unexpected local conn call")
		},
	)

	err := executor.Execute(context.Background(), chatID)
	require.NoError(t, err)
}

func TestBuildDynamicTools(t *testing.T) {
	t.Parallel()

	providerConfig, err := json.Marshal(fantasy.ProviderOptions{
		fantasyopenai.Name: &fantasyopenai.ProviderOptions{User: fantasy.Opt("tester")},
	})
	require.NoError(t, err)

	tools := chatexec.BuildDynamicToolsForTest(
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		[]agentsdk.ChatRunnerToolDefinition{
			{
				Name:           "dynamic_lookup",
				Description:    "Look up records",
				InputSchema:    json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
				ProviderConfig: providerConfig,
			},
			{
				Name:        "bad_schema",
				Description: "Tool with malformed schema",
				InputSchema: json.RawMessage("not-json"),
			},
		},
	)
	require.Len(t, tools, 2)

	lookup := tools[0].Info()
	require.Equal(t, "dynamic_lookup", lookup.Name)
	require.Equal(t, "Look up records", lookup.Description)
	require.Contains(t, lookup.Parameters, "query")
	require.Equal(t, []string{"query"}, lookup.Required)

	providerOptions := tools[0].ProviderOptions()
	decoded, ok := providerOptions[fantasyopenai.Name].(*fantasyopenai.ProviderOptions)
	require.True(t, ok)
	require.NotNil(t, decoded.User)
	require.Equal(t, "tester", *decoded.User)

	resp, err := tools[0].Run(context.Background(), fantasy.ToolCall{ID: "call-1", Name: "dynamic_lookup", Input: `{}`})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "dynamic tool called in chatloop")

	badSchema := tools[1].Info()
	require.Equal(t, "bad_schema", badSchema.Name)
	require.Nil(t, badSchema.Parameters)
	require.Nil(t, badSchema.Required)
}

func TestBuildListTemplatesTool_Success(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	leaseEpoch := int64(17)
	templateOneID := uuid.New()
	templateTwoID := uuid.New()
	client := &mockChatRunnerClient{
		listTemplatesResp: agentsdk.ChatRunnerListTemplatesResponse{
			Templates: []agentsdk.ChatRunnerTemplate{
				{
					ID:          templateOneID,
					Name:        "go-dev",
					DisplayName: " Go Dev ",
					Description: " Build Go services ",
				},
				{
					ID:          templateTwoID,
					Name:        "blank",
					DisplayName: "   ",
					Description: "",
				},
			},
			TotalCount: 21,
			Page:       2,
			PageSize:   10,
		},
	}

	tool := chatexec.BuildListTemplatesToolForTest(client, chatID, leaseEpoch)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  "list_templates",
		Input: `{"query":"dev","page":2}`,
	})
	require.NoError(t, err)
	require.False(t, resp.IsError)

	calls := client.listTemplatesCallsSnapshot()
	require.Len(t, calls, 1)
	require.Equal(t, agentsdk.ChatRunnerListTemplatesRequest{
		ChatID:     chatID,
		LeaseEpoch: leaseEpoch,
		Query:      "dev",
		Page:       2,
	}, calls[0])
	require.JSONEq(t, fmt.Sprintf(`{
		"templates": [
			{
				"id": %q,
				"name": "go-dev",
				"display_name": "Go Dev",
				"description": "Build Go services"
			},
			{
				"id": %q,
				"name": "blank"
			}
		],
		"count": 2,
		"page": 2,
		"total_pages": 3,
		"total_count": 21
	}`, templateOneID.String(), templateTwoID.String()), resp.Content)
}

func TestBuildListTemplatesTool_Error(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	leaseEpoch := int64(18)
	client := &mockChatRunnerClient{listTemplatesErr: xerrors.New("list failed")}

	tool := chatexec.BuildListTemplatesToolForTest(client, chatID, leaseEpoch)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  "list_templates",
		Input: `{"query":"dev","page":3}`,
	})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Equal(t, "list failed", resp.Content)

	calls := client.listTemplatesCallsSnapshot()
	require.Len(t, calls, 1)
	require.Equal(t, agentsdk.ChatRunnerListTemplatesRequest{
		ChatID:     chatID,
		LeaseEpoch: leaseEpoch,
		Query:      "dev",
		Page:       3,
	}, calls[0])
}

func TestBuildReadTemplateTool_Success(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	leaseEpoch := int64(19)
	templateID := uuid.New()
	client := &mockChatRunnerClient{
		readTemplateResp: agentsdk.ChatRunnerReadTemplateResponse{
			Template: agentsdk.ChatRunnerTemplate{
				ID:          templateID,
				Name:        "go-dev",
				DisplayName: " Go Dev ",
				Description: " Build Go services ",
			},
			Parameters: []agentsdk.ChatRunnerTemplateParameter{
				{
					Name:         "region",
					Type:         "string",
					Required:     true,
					DisplayName:  " Region ",
					Description:  " Pick a region ",
					DefaultValue: "us-east-1",
					Mutable:      true,
					Options: []agentsdk.ChatRunnerTemplateParameterOption{
						{Name: "US East 1", Description: " Virginia ", Value: "us-east-1"},
						{Name: "US West 2", Value: "us-west-2"},
					},
				},
				{
					Name:        "replicas",
					Type:        "number",
					Required:    false,
					DisplayName: "   ",
					Description: "",
				},
			},
		},
	}

	tool := chatexec.BuildReadTemplateToolForTest(client, chatID, leaseEpoch)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  "read_template",
		Input: fmt.Sprintf(`{"template_id": %q}`, " "+templateID.String()+" "),
	})
	require.NoError(t, err)
	require.False(t, resp.IsError)

	calls := client.readTemplateCallsSnapshot()
	require.Len(t, calls, 1)
	require.Equal(t, agentsdk.ChatRunnerReadTemplateRequest{
		ChatID:     chatID,
		LeaseEpoch: leaseEpoch,
		TemplateID: templateID,
	}, calls[0])
	require.JSONEq(t, fmt.Sprintf(`{
		"template": {
			"id": %q,
			"name": "go-dev",
			"display_name": "Go Dev",
			"description": "Build Go services"
		},
		"parameters": [
			{
				"name": "region",
				"type": "string",
				"required": true,
				"display_name": "Region",
				"description": "Pick a region",
				"default": "us-east-1",
				"mutable": true,
				"options": [
					{
						"name": "US East 1",
						"description": "Virginia",
						"value": "us-east-1"
					},
					{
						"name": "US West 2",
						"value": "us-west-2"
					}
				]
			},
			{
				"name": "replicas",
				"type": "number",
				"required": false
			}
		]
	}`, templateID.String()), resp.Content)
}

func TestBuildReadTemplateTool_Error(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	leaseEpoch := int64(20)
	templateID := uuid.New()
	client := &mockChatRunnerClient{readTemplateErr: xerrors.New("read failed")}

	tool := chatexec.BuildReadTemplateToolForTest(client, chatID, leaseEpoch)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  "read_template",
		Input: fmt.Sprintf(`{"template_id": %q}`, templateID.String()),
	})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Equal(t, "read failed", resp.Content)

	calls := client.readTemplateCallsSnapshot()
	require.Len(t, calls, 1)
	require.Equal(t, agentsdk.ChatRunnerReadTemplateRequest{
		ChatID:     chatID,
		LeaseEpoch: leaseEpoch,
		TemplateID: templateID,
	}, calls[0])
}

func TestBuildReadTemplateTool_InvalidTemplateID(t *testing.T) {
	t.Parallel()

	tool := chatexec.BuildReadTemplateToolForTest(&mockChatRunnerClient{}, uuid.New(), 21)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  "read_template",
		Input: `{"template_id":"not-a-uuid"}`,
	})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "invalid template_id")
}

func TestUnsupportedBuiltinToolsAreIgnored(t *testing.T) {
	t.Parallel()

	localTool, ok, err := chatexec.LocalToolFromDefinitionForTest(
		agentsdk.ChatRunnerToolDefinition{Name: "create_workspace"},
		nil,
	)
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, localTool)

	controlPlaneTool, ok := chatexec.ControlPlaneToolFromDefinitionForTest(
		agentsdk.ChatRunnerToolDefinition{Name: "create_workspace"},
		nil,
		uuid.New(),
		1,
	)
	require.False(t, ok)
	require.Nil(t, controlPlaneTool)
}
