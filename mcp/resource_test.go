package mcp_test

import (
	"encoding/json"
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	codermcp "github.com/coder/coder/v2/mcp"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

// These tests are dependent on the state of the coder server.
// Running them in parallel is prone to racy behavior.
// nolint:tparallel,paralleltest
func TestCoderResources(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-linux due to pty issues")
	}
	ctx := testutil.Context(t, testutil.WaitLong)
	// Given: a coder server, workspace, and agent.
	client, store := coderdtest.NewWithDatabase(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	// Given: a member user with which to test the resources.
	memberClient, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	// Given: a workspace with an agent.
	r := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
		OrganizationID: owner.OrganizationID,
		OwnerID:        member.ID,
	}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
		agents[0].Apps = []*proto.App{
			{
				Slug: "some-agent-app",
			},
		}
		return agents
	}).Do()

	agt := agenttest.New(t, client.URL, r.AgentToken)
	t.Cleanup(func() {
		_ = agt.Close()
	})
	_ = coderdtest.NewWorkspaceAgentWaiter(t, client, r.Workspace.ID).Wait()

	// Given: a MCP server listening on a pty.
	pty := ptytest.New(t)
	mcpSrv, closeSrv := startTestMCPServer(ctx, t, pty.Input(), pty.Output())
	t.Cleanup(func() {
		_ = closeSrv()
	})

	logger := slogtest.Make(t, nil)
	agentClient := agentsdk.New(memberClient.URL)
	codermcp.RegisterResources(mcpSrv, codermcp.ToolDeps{
		Client:        memberClient,
		Logger:        &logger,
		AppStatusSlug: "some-agent-app",
		AgentClient:   agentClient,
	})

	t.Run("resources_list", func(t *testing.T) {
		// When: the resources/templates/list endpoint is called
		rListReq := makeResourceJSONRPCRequest(t, "resources/templates/list", "")
		pty.WriteLine(rListReq)
		_ = pty.ReadLine(ctx) // skip the echo

		// Then: the response is a valid ResourcesListResponse
		resp := struct {
			Result struct {
				ResourceTemplates []struct {
					URITemplate string `json:"uriTemplate"`
					Name        string `json:"name"`
					Description string `json:"description,omitempty"`
					MIMEType    string `json:"mimeType,omitempty"`
				} `json:"resourceTemplates"`
			} `json:"result"`
		}{}
		response := pty.ReadLine(ctx)
		err := json.Unmarshal([]byte(response), &resp)
		require.NoError(t, err)

		// Then: the response contains our expected resources
		var hasUserResource, hasTemplatesResource, hasWorkspacesResource, hasWorkspaceResource bool
		for _, resource := range resp.Result.ResourceTemplates {
			switch resource.URITemplate {
			case "coder://user/{id}":
				hasUserResource = true
			case "coder://templates":
				hasTemplatesResource = true
			case "coder://workspaces{?limit,offset,owner}":
				hasWorkspacesResource = true
			case "coder://workspace/{id}":
				hasWorkspaceResource = true
			default:
				assert.Failf(t, "unexpected resource %q", resource.URITemplate)
			}
		}

		require.True(t, hasUserResource, "expected coder://user resource")
		require.True(t, hasTemplatesResource, "expected coder://templates resource")
		require.True(t, hasWorkspacesResource, "expected coder://workspaces resource")
		require.True(t, hasWorkspaceResource, "expected coder://workspace/{id} resource")
	})

	t.Run("resources_read_user", func(t *testing.T) {
		t.Run("me", func(t *testing.T) {
			// When: the resources/read endpoint is called for user
			rReadReq := makeResourceJSONRPCRequest(t, "resources/read", "coder://user/me")
			pty.WriteLine(rReadReq)
			_ = pty.ReadLine(ctx) // skip the echo

			// Then: the response contains the user data
			response := pty.ReadLine(ctx)
			actual := unmarshalFromResourceReadResult[codersdk.User](t, response)

			// Check user data
			expected, err := memberClient.User(ctx, codersdk.Me)
			require.NoError(t, err)
			if diff := cmp.Diff(expected, actual); diff != "" {
				t.Errorf("user mismatch (-want +got):\n%s", diff)
			}
		})

		t.Run("id", func(t *testing.T) {
			// When: the resources/read endpoint is called for user
			rReadReq := makeResourceJSONRPCRequest(t, "resources/read", "coder://user/"+member.ID.String())
			pty.WriteLine(rReadReq)
			_ = pty.ReadLine(ctx) // skip the echo

			// Then: the response contains the user data
			response := pty.ReadLine(ctx)
			actual := unmarshalFromResourceReadResult[codersdk.User](t, response)

			// Check user data
			expected, err := memberClient.User(ctx, codersdk.Me)
			require.NoError(t, err)
			if diff := cmp.Diff(expected, actual); diff != "" {
				t.Errorf("user mismatch (-want +got):\n%s", diff)
			}
		})

		t.Run("NotFound", func(t *testing.T) {
			// When: the resources/read endpoint is called for a user for which we do
			// not have permission (a.k.a. not found)
			rReadReq := makeResourceJSONRPCRequest(t, "resources/read", "coder://user/"+owner.UserID.String())
			pty.WriteLine(rReadReq)
			_ = pty.ReadLine(ctx) // skip the echo

			// Then: the response is an error
			response := pty.ReadLine(ctx)
			err := unmarshalErrorResponse(response)
			require.ErrorContains(t, err, "must be an existing uuid or username")
		})
	})

	t.Run("resources_read_templates", func(t *testing.T) {
		// When: the resources/read endpoint is called for templates
		rReadReq := makeResourceJSONRPCRequest(t, "resources/read", "coder://templates")
		pty.WriteLine(rReadReq)
		_ = pty.ReadLine(ctx) // skip the echo

		// Then: the response contains the templates data
		response := pty.ReadLine(ctx)
		templatesData := unmarshalFromResourceReadResult[[]codersdk.Template](t, response)

		// Check templates data
		expected, err := memberClient.Templates(ctx, codersdk.TemplateFilter{})
		require.NoError(t, err)
		require.Len(t, templatesData, 1)
		require.Equal(t, expected[0].ID, templatesData[0].ID)
		require.Equal(t, expected[0].Name, templatesData[0].Name)
	})

	t.Run("resources_read_workspaces", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			// When: the resources/read endpoint is called for workspaces with no query parameters
			rReadReq := makeResourceJSONRPCRequest(t, "resources/read", "coder://workspaces")
			pty.WriteLine(rReadReq)
			_ = pty.ReadLine(ctx) // skip the echo

			// Then: the response contains the workspaces data
			response := pty.ReadLine(ctx)
			workspacesData := unmarshalFromResourceReadResult[[]codersdk.ReducedWorkspace](t, response)

			// Check workspaces data
			require.Len(t, workspacesData, 1)
			require.Equal(t, r.Workspace.ID, workspacesData[0].ID)
		})
		t.Run("filter", func(t *testing.T) {
			// When: the resources/read endpoint is called for workspaces with query parameters
			rReadReq := makeResourceJSONRPCRequest(t, "resources/read", "coder://workspaces?offset=1&limit=1&owner=me")
			pty.WriteLine(rReadReq)
			_ = pty.ReadLine(ctx) // skip the echo

			// Then: the response should contain no workspaces as we paginated past
			response := pty.ReadLine(ctx)
			workspacesData := unmarshalFromResourceReadResult[[]codersdk.ReducedWorkspace](t, response)

			// Check workspaces data
			require.Len(t, workspacesData, 0)
		})
	})

	t.Run("resources_read_workspace", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			// When: the resources/read endpoint is called for a specific workspace
			rReadReq := makeResourceJSONRPCRequest(t, "resources/read", "coder://workspace/"+r.Workspace.ID.String())
			pty.WriteLine(rReadReq)
			_ = pty.ReadLine(ctx) // skip the echo

			// Then: the response contains the workspace data
			response := pty.ReadLine(ctx)
			workspaceData := unmarshalFromResourceReadResult[codersdk.Workspace](t, response)

			// Check workspace data
			expected, err := memberClient.Workspace(ctx, r.Workspace.ID)
			require.NoError(t, err)
			require.Equal(t, expected.ID, workspaceData.ID)
			require.Equal(t, expected.Name, workspaceData.Name)
		})

		t.Run("notfound", func(t *testing.T) {
			// When: the resources/read endpoint is called for a non-existent workspace
			rReadReq := makeResourceJSONRPCRequest(t, "resources/read", "coder://workspace/"+uuid.New().String())
			pty.WriteLine(rReadReq)
			_ = pty.ReadLine(ctx) // skip the echo

			// Then: the response contains an error
			response := pty.ReadLine(ctx)
			require.Contains(t, response, "error")
			require.Contains(t, response, "not found")
		})
	})
}

// makeResourceJSONRPCRequest is a helper function that makes a JSON RPC request for resources.
func makeResourceJSONRPCRequest(t *testing.T, method string, uri string) string {
	t.Helper()
	req := struct {
		ID      string `json:"id"`
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  struct {
			URI string `json:"uri"`
		} `json:"params"`
	}{
		ID:      "1",
		JSONRPC: "2.0",
		Method:  method,
		Params: struct {
			URI string `json:"uri"`
		}{
			URI: uri,
		},
	}
	bs, err := json.Marshal(req)
	require.NoError(t, err, "failed to marshal JSON RPC request")
	return string(bs)
}

func unmarshalErrorResponse(raw string) error {
	var errResp struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &errResp); err != nil {
		return nil
	}

	if errResp.Error.Message == "" {
		return nil
	}

	return xerrors.New(errResp.Error.Message)
}

// unmarshalFromResourceReadResult is a helper to unmarshal the resources/read response
func unmarshalFromResourceReadResult[T any](t *testing.T, raw string) T {
	t.Helper()

	// First check if there is an error response in the raw string.
	if err := unmarshalErrorResponse(raw); err != nil {
		assert.NoError(t, err, "expected no error")
		t.FailNow()
	}

	var resp struct {
		JSONRPC string `json:"jsonrpc"`
		ID      string `json:"id"`
		Result  struct {
			Contents []struct {
				URI      string `json:"uri"`
				MIMEType string `json:"mimeType"`
				Text     string `json:"text"`
			} `json:"contents"`
		} `json:"result"`
	}

	require.NoError(t, json.Unmarshal([]byte(raw), &resp), "failed to unmarshal JSON RPC response")
	require.NotEmpty(t, resp.Result.Contents, "expected non-empty contents")

	var actual T
	require.NoError(t, json.Unmarshal([]byte(resp.Result.Contents[0].Text), &actual), "failed to unmarshal text content")
	return actual
}
