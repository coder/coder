package coderd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/coderdtest"
	mcpserver "github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

type quotaMCPTransport struct {
	base  http.RoundTripper
	token string
}

func (q *quotaMCPTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+q.token)
	return q.base.RoundTrip(clone)
}

func quotaMCPClient(ctx context.Context, t *testing.T, client *codersdk.Client) *mcp.ClientSession {
	t.Helper()
	httpClient := coderdtest.NewIsolatedHTTPClient(nil)
	httpClient.Transport = &quotaMCPTransport{base: httpClient.Transport, token: client.SessionToken()}
	session, err := mcp.NewClient(&mcp.Implementation{Name: uuid.NewString(), Version: "1.0.0"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: client.URL.String() + mcpserver.MCPEndpoint, HTTPClient: httpClient}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func quotaMCPCall(ctx context.Context, client *mcp.ClientSession, name string, args any) (codersdk.WorkspaceExecutionSession, error) {
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return codersdk.WorkspaceExecutionSession{}, err
	}
	if result.IsError {
		return codersdk.WorkspaceExecutionSession{}, xerrors.Errorf("MCP acquisition: %v", result.Content)
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return codersdk.WorkspaceExecutionSession{}, err
	}
	var receipt codersdk.WorkspaceExecutionSession
	err = json.Unmarshal(raw, &receipt)
	return receipt, err
}

func TestMCPHTTP_WorkspaceAcquisitionQuotaFailure(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	defer cancel()
	client, closer, api, user := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{DeploymentValues: coderdtest.DeploymentValues(t, func(dv *codersdk.DeploymentValues) {
			dv.Experiments = []string{string(codersdk.ExperimentMCPServerHTTP)}
		})}, UserWorkspaceQuota: 1,
		LicenseOptions: &coderdenttest.LicenseOptions{Features: license.Features{codersdk.FeatureTemplateRBAC: 1, codersdk.FeatureMultipleOrganizations: 1}},
	})
	defer closer.Close()
	coderdtest.NewProvisionerDaemon(t, api.AGPL)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete, ProvisionPlan: []*proto.Response{{Type: &proto.Response_Plan{Plan: &proto.PlanComplete{DailyCost: 1}}}},
		ProvisionGraph: []*proto.Response{{Type: &proto.Response_Graph{Graph: &proto.GraphComplete{Resources: []*proto.Resource{{Name: "quota", Type: "test", DailyCost: 1}}}}}},
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	session := quotaMCPClient(ctx, t, client)
	args := toolsdk.AcquireWorkspaceExecutionArgs{OrganizationID: user.OrganizationID.String(), AcquireWorkspaceExecutionRequest: codersdk.AcquireWorkspaceExecutionRequest{RequestID: uuid.New(), OwnerID: user.UserID, Create: &codersdk.CreateWorkspaceRequest{Name: "quota-" + uuid.NewString()[:8], TemplateID: template.ID}, LeaseExpiresAt: time.Now().Add(time.Hour), Retained: new(true)}}
	receipt, err := quotaMCPCall(ctx, session, toolsdk.ToolNameAcquireWorkspaceExecution, args)
	require.NoError(t, err)
	build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, *receipt.AcquisitionBuildID)
	require.Equal(t, codersdk.WorkspaceStatusFailed, build.Status)
	require.Contains(t, build.Job.Error, "quota")
	status, err := quotaMCPCall(ctx, session, toolsdk.ToolNameGetWorkspaceExecutionSession, toolsdk.GetWorkspaceExecutionSessionArgs{OrganizationID: user.OrganizationID.String(), SessionID: receipt.ID})
	require.NoError(t, err)
	require.Equal(t, codersdk.WorkspaceStatusFailed, status.AcquisitionBuild.Status)
	require.Equal(t, build.Job.Error, status.AcquisitionBuild.Error)
	require.Equal(t, string(build.Job.ErrorCode), status.AcquisitionBuild.ErrorCode)
	require.Equal(t, receipt.AcquisitionBuildID, status.AcquisitionBuildID)
	otherOrg := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})
	_, err = quotaMCPCall(ctx, session, toolsdk.ToolNameGetWorkspaceExecutionSession, toolsdk.GetWorkspaceExecutionSessionArgs{OrganizationID: otherOrg.ID.String(), SessionID: receipt.ID})
	require.Error(t, err)
	wrong := args
	wrong.OrganizationID = otherOrg.ID.String()
	wrong.RequestID = uuid.New()
	_, err = quotaMCPCall(ctx, session, toolsdk.ToolNameAcquireWorkspaceExecution, wrong)
	require.Error(t, err)

	replay, err := quotaMCPCall(ctx, session, toolsdk.ToolNameAcquireWorkspaceExecution, args)
	require.NoError(t, err)
	require.Equal(t, receipt.ID, replay.ID)
	workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{Owner: codersdk.Me})
	require.NoError(t, err)
	require.Len(t, workspaces.Workspaces, 1)
}
