package chattool_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"charm.land/fantasy"

	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/chatd/chattool"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

// TestFromToolSDK verifies that FromToolSDK correctly adapts a
// toolsdk.GenericTool into a fantasy.AgentTool. It checks schema
// mapping, name/description pass-through, successful invocation,
// error handling, and JSON argument round-tripping.
func TestFromToolSDK(t *testing.T) {
	t.Parallel()

	t.Run("SchemaMapping", func(t *testing.T) {
		t.Parallel()

		// Build a GenericTool with known schema properties and
		// required fields.
		gt := toolsdk.GenericTool{
			Tool: aisdk.Tool{
				Name:        "test_schema_tool",
				Description: "A tool for testing schema mapping.",
				Schema: aisdk.Schema{
					Properties: map[string]any{
						"name": map[string]any{
							"type":        "string",
							"description": "The user name.",
						},
						"count": map[string]any{
							"type":        "integer",
							"description": "How many items.",
						},
					},
					Required: []string{"name"},
				},
			},
			Handler: func(_ context.Context, _ toolsdk.Deps, _ json.RawMessage) (json.RawMessage, error) {
				return json.RawMessage(`{}`), nil
			},
		}

		depsFunc := func() (toolsdk.Deps, error) {
			return toolsdk.Deps{}, nil
		}

		tool := chattool.FromToolSDK(gt, depsFunc)

		info := tool.Info()
		assert.Equal(t, "test_schema_tool", info.Name)
		assert.Equal(t, "A tool for testing schema mapping.", info.Description)

		// The aisdk.Schema.Properties should map to
		// fantasy.ToolInfo.Parameters.
		require.NotNil(t, info.Parameters)
		require.Contains(t, info.Parameters, "name")
		require.Contains(t, info.Parameters, "count")

		// Required fields should pass through.
		require.Equal(t, []string{"name"}, info.Required)
	})

	t.Run("NameAndDescription", func(t *testing.T) {
		t.Parallel()

		gt := toolsdk.GenericTool{
			Tool: aisdk.Tool{
				Name:        "my_tool",
				Description: "Does something useful.",
				Schema: aisdk.Schema{
					Properties: map[string]any{},
					Required:   []string{},
				},
			},
			Handler: func(_ context.Context, _ toolsdk.Deps, _ json.RawMessage) (json.RawMessage, error) {
				return json.RawMessage(`{}`), nil
			},
		}

		depsFunc := func() (toolsdk.Deps, error) {
			return toolsdk.Deps{}, nil
		}

		tool := chattool.FromToolSDK(gt, depsFunc)
		info := tool.Info()
		assert.Equal(t, "my_tool", info.Name)
		assert.Equal(t, "Does something useful.", info.Description)
	})

	t.Run("SuccessfulInvocation", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		gt := toolsdk.GenericTool{
			Tool: aisdk.Tool{
				Name:        "echo_tool",
				Description: "Echoes back a greeting.",
				Schema: aisdk.Schema{
					Properties: map[string]any{
						"message": map[string]any{
							"type": "string",
						},
					},
					Required: []string{"message"},
				},
			},
			Handler: func(_ context.Context, _ toolsdk.Deps, args json.RawMessage) (json.RawMessage, error) {
				var input struct {
					Message string `json:"message"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return nil, err
				}
				result := map[string]string{"echo": input.Message}
				return json.Marshal(result)
			},
		}

		depsFunc := func() (toolsdk.Deps, error) {
			return toolsdk.Deps{}, nil
		}

		tool := chattool.FromToolSDK(gt, depsFunc)

		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "echo_tool",
			Input: `{"message":"hello"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)

		// The content should be the JSON-encoded handler result.
		var result map[string]string
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		assert.Equal(t, "hello", result["echo"])
	})

	t.Run("ErrorHandling", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		gt := toolsdk.GenericTool{
			Tool: aisdk.Tool{
				Name:        "failing_tool",
				Description: "Always fails.",
				Schema: aisdk.Schema{
					Properties: map[string]any{},
					Required:   []string{},
				},
			},
			Handler: func(_ context.Context, _ toolsdk.Deps, _ json.RawMessage) (json.RawMessage, error) {
				return nil, assert.AnError
			},
		}

		depsFunc := func() (toolsdk.Deps, error) {
			return toolsdk.Deps{}, nil
		}

		tool := chattool.FromToolSDK(gt, depsFunc)

		// A Go error from the handler should become a ToolResponse
		// with IsError=true and a nil Go error.
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-2",
			Name:  "failing_tool",
			Input: `{}`,
		})
		require.NoError(t, err, "adapter should not return a Go error")
		assert.True(t, resp.IsError, "response should be marked as error")
		assert.Contains(t, resp.Content, "assert.AnError")
	})

	t.Run("JSONRoundTrip", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		// This test verifies that the ToolCall.Input JSON string
		// is correctly forwarded as json.RawMessage to the handler.
		var capturedArgs json.RawMessage

		gt := toolsdk.GenericTool{
			Tool: aisdk.Tool{
				Name:        "capture_tool",
				Description: "Captures args for inspection.",
				Schema: aisdk.Schema{
					Properties: map[string]any{
						"alpha": map[string]any{"type": "string"},
						"beta":  map[string]any{"type": "integer"},
					},
					Required: []string{"alpha", "beta"},
				},
			},
			Handler: func(_ context.Context, _ toolsdk.Deps, args json.RawMessage) (json.RawMessage, error) {
				capturedArgs = args
				return json.RawMessage(`{"ok":true}`), nil
			},
		}

		depsFunc := func() (toolsdk.Deps, error) {
			return toolsdk.Deps{}, nil
		}

		tool := chattool.FromToolSDK(gt, depsFunc)

		inputJSON := `{"alpha":"foo","beta":42}`
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-3",
			Name:  "capture_tool",
			Input: inputJSON,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)

		// Verify the handler received exactly the input we sent.
		var sent, received map[string]any
		require.NoError(t, json.Unmarshal([]byte(inputJSON), &sent))
		require.NoError(t, json.Unmarshal(capturedArgs, &received))
		assert.Equal(t, sent, received)
	})
}

// TestAdapterListTemplates verifies the full adapter path for the
// ChatListTemplates tool against a real coderdtest server.
func TestAdapterListTemplates(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitMedium)

	// Stand up a real server and create a template so there's
	// something to list.
	client := coderdtest.New(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	memberClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	// Create a template so the list is non-empty.
	version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

	deps, err := toolsdk.NewDeps(memberClient)
	require.NoError(t, err)

	depsFunc := func() (toolsdk.Deps, error) {
		return deps, nil
	}

	tool := chattool.FromToolSDK(toolsdk.ChatListTemplates.Generic(), depsFunc)

	info := tool.Info()
	assert.Equal(t, toolsdk.ChatListTemplates.Name, info.Name)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-lt-1",
		Name:  info.Name,
		Input: `{}`,
	})
	require.NoError(t, err)
	require.False(t, resp.IsError, "unexpected error: %s", resp.Content)

	// Parse the JSON response and verify we got at least one
	// template back.
	var result json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))

	// The raw JSON should mention the template we created.
	assert.Contains(t, resp.Content, version.Name)
}

// TestAdapterReadFile verifies the adapter for ChatReadFile against
// a workspace agent. It writes a file via the agent and reads it
// back through the adapted tool.
func TestAdapterReadFile(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	client, store := coderdtest.NewWithDatabase(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	memberClient, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	// Build a workspace with an agent.
	ws, _ := createWorkspaceWithAgent(t, store, client, owner.OrganizationID, member.ID)

	conn := connectToAgent(t, ctx, memberClient, ws)

	deps, err := toolsdk.NewDeps(memberClient, toolsdk.WithAgentConn(func() workspacesdk.AgentConn {
		return conn
	}))
	require.NoError(t, err)

	depsFunc := func() (toolsdk.Deps, error) {
		return deps, nil
	}

	tool := chattool.FromToolSDK(toolsdk.ChatReadFile.Generic(), depsFunc)

	// Write a known file via the agent so we can read it back.
	writeFileViaAgent(t, ctx, conn, "/tmp/test-chattool.txt", "line one\nline two\nline three\n")

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:   "call-rf-1",
		Name: tool.Info().Name,
		Input: `{"path":"/tmp/test-chattool.txt"}`,
	})
	require.NoError(t, err)
	require.False(t, resp.IsError, "unexpected error: %s", resp.Content)

	// The response should contain file content with line numbers,
	// file_size, and total_lines.
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	assert.Contains(t, resp.Content, "line one")
	assert.Contains(t, result, "file_size")
	assert.Contains(t, result, "total_lines")
}

// TestAdapterExecute verifies the adapter for ChatExecute by running
// a simple echo command through a workspace agent.
func TestAdapterExecute(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	client, store := coderdtest.NewWithDatabase(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	memberClient, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	ws, _ := createWorkspaceWithAgent(t, store, client, owner.OrganizationID, member.ID)

	conn := connectToAgent(t, ctx, memberClient, ws)

	deps, err := toolsdk.NewDeps(memberClient, toolsdk.WithAgentConn(func() workspacesdk.AgentConn {
		return conn
	}))
	require.NoError(t, err)

	depsFunc := func() (toolsdk.Deps, error) {
		return deps, nil
	}

	tool := chattool.FromToolSDK(toolsdk.ChatExecute.Generic(), depsFunc)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:   "call-ex-1",
		Name: tool.Info().Name,
		Input: `{"command":"echo hello"}`,
	})
	require.NoError(t, err)
	require.False(t, resp.IsError, "unexpected error: %s", resp.Content)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	assert.Equal(t, true, result["success"])
	assert.Contains(t, result["output"], "hello")
}

// TestAdapterErrorMapping verifies the error mapping contract: a
// toolsdk handler that returns (result, error) maps to
// (ToolResponse{IsError:true}, nil) in the fantasy adapter, and a
// handler returning (result, nil) maps to (ToolResponse{Content:
// json}, nil).
func TestAdapterErrorMapping(t *testing.T) {
	t.Parallel()

	t.Run("HandlerError", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		gt := toolsdk.GenericTool{
			Tool: aisdk.Tool{
				Name:        "error_mapper",
				Description: "Returns an error from the handler.",
				Schema: aisdk.Schema{
					Properties: map[string]any{},
					Required:   []string{},
				},
			},
			Handler: func(_ context.Context, _ toolsdk.Deps, _ json.RawMessage) (json.RawMessage, error) {
				// Simulate a real toolsdk error: some handlers
				// return partial JSON alongside an error.
				return json.RawMessage(`{"partial":"data"}`), assert.AnError
			},
		}

		depsFunc := func() (toolsdk.Deps, error) {
			return toolsdk.Deps{}, nil
		}

		tool := chattool.FromToolSDK(gt, depsFunc)

		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-err-1",
			Name:  "error_mapper",
			Input: `{}`,
		})
		// The adapter must absorb the Go error.
		require.NoError(t, err, "adapter must not propagate Go errors")
		assert.True(t, resp.IsError, "response must be flagged as error")
	})

	t.Run("HandlerSuccess", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		gt := toolsdk.GenericTool{
			Tool: aisdk.Tool{
				Name:        "success_mapper",
				Description: "Returns a successful result.",
				Schema: aisdk.Schema{
					Properties: map[string]any{},
					Required:   []string{},
				},
			},
			Handler: func(_ context.Context, _ toolsdk.Deps, _ json.RawMessage) (json.RawMessage, error) {
				return json.RawMessage(`{"status":"ok"}`), nil
			},
		}

		depsFunc := func() (toolsdk.Deps, error) {
			return toolsdk.Deps{}, nil
		}

		tool := chattool.FromToolSDK(gt, depsFunc)

		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-ok-1",
			Name:  "success_mapper",
			Input: `{}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError, "successful result must not be flagged as error")
		assert.JSONEq(t, `{"status":"ok"}`, resp.Content)
	})

	t.Run("DepsError", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		gt := toolsdk.GenericTool{
			Tool: aisdk.Tool{
				Name:        "deps_fail_tool",
				Description: "Tool whose deps func fails.",
				Schema: aisdk.Schema{
					Properties: map[string]any{},
					Required:   []string{},
				},
			},
			Handler: func(_ context.Context, _ toolsdk.Deps, _ json.RawMessage) (json.RawMessage, error) {
				return json.RawMessage(`{}`), nil
			},
		}

		depsFunc := func() (toolsdk.Deps, error) {
			return toolsdk.Deps{}, assert.AnError
		}

		tool := chattool.FromToolSDK(gt, depsFunc)

		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-depsfail-1",
			Name:  "deps_fail_tool",
			Input: `{}`,
		})
		// A deps resolution failure should also be absorbed as an
		// error response, not a Go-level error.
		require.NoError(t, err, "adapter must not propagate Go errors")
		assert.True(t, resp.IsError, "deps failure must be flagged as error")
		assert.Contains(t, resp.Content, "assert.AnError")
	})
}

// createWorkspaceWithAgent is a test helper that creates a workspace
// with a running agent. It mirrors the pattern used in
// toolsdk_test.go.
//
// nolint:revive // unused for now — this is the RED phase.
func createWorkspaceWithAgent(
	t *testing.T,
	store database.Store,
	client *codersdk.Client,
	orgID, ownerID uuid.UUID,
) (codersdk.Workspace, string) {
	t.Helper()
	r := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
		OrganizationID: orgID,
		OwnerID:        ownerID,
	}).WithAgent().Do()

	_ = agenttest.New(t, client.URL, r.AgentToken)
	coderdtest.NewWorkspaceAgentWaiter(t, client, r.Workspace.ID).Wait()

	ctx := testutil.Context(t, testutil.WaitShort)
	ws, err := client.Workspace(ctx, r.Workspace.ID)
	require.NoError(t, err)
	return ws, r.AgentToken
}

// connectToAgent establishes a workspace agent connection.
func connectToAgent(
	t *testing.T,
	ctx context.Context,
	client *codersdk.Client,
	ws codersdk.Workspace,
) workspacesdk.AgentConn {
	t.Helper()
	require.NotEmpty(t, ws.LatestBuild.Resources)
	require.NotEmpty(t, ws.LatestBuild.Resources[0].Agents)
	agentID := ws.LatestBuild.Resources[0].Agents[0].ID

	wsClient := workspacesdk.New(client)
	conn, err := wsClient.DialAgent(ctx, agentID, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.True(t, conn.AwaitReachable(ctx))
	return conn
}

// writeFileViaAgent writes a file through the agent connection.
func writeFileViaAgent(
	t *testing.T,
	ctx context.Context,
	conn workspacesdk.AgentConn,
	path, content string,
) {
	t.Helper()
	err := conn.WriteFile(ctx, path, strings.NewReader(content))
	require.NoError(t, err)
}
