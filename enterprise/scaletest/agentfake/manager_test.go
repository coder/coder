package agentfake_test

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/scaletest/agentfake"
	"github.com/coder/coder/v2/testutil"
)

// fakeExternalAgentClient is an in-package fake for the
// ExternalAgentClient interface used by
// Manager.EnumerateExternalAgents. Tests populate workspaces /
// credentials / workspacesErr before calling the Manager.
type fakeExternalAgentClient struct {
	// workspaces, in the order Workspaces() should return them. Each
	// call returns up to filter.Limit entries starting at filter.Offset
	// to model pagination, matching real coderd behavior.
	workspaces []codersdk.Workspace
	// credentials, keyed by "{workspaceID}/{agentName}". A nil entry
	// causes WorkspaceExternalAgentCredentials to error with notFoundErr.
	credentials map[string]codersdk.ExternalAgentCredentials

	// workspacesErr, if non-nil, is returned from every Workspaces call.
	workspacesErr error
}

func (f *fakeExternalAgentClient) Workspaces(_ context.Context, filter codersdk.WorkspaceFilter) (codersdk.WorkspacesResponse, error) {
	if f.workspacesErr != nil {
		return codersdk.WorkspacesResponse{}, f.workspacesErr
	}
	start := filter.Offset
	if start > len(f.workspaces) {
		start = len(f.workspaces)
	}
	end := start + filter.Limit
	if end > len(f.workspaces) {
		end = len(f.workspaces)
	}
	page := f.workspaces[start:end]
	return codersdk.WorkspacesResponse{
		Workspaces: page,
		Count:      len(f.workspaces),
	}, nil
}

func (f *fakeExternalAgentClient) WorkspaceExternalAgentCredentials(_ context.Context, wsID uuid.UUID, agentName string) (codersdk.ExternalAgentCredentials, error) {
	key := wsID.String() + "/" + agentName
	creds, ok := f.credentials[key]
	if !ok {
		return codersdk.ExternalAgentCredentials{}, xerrors.Errorf("no credentials for %s", key)
	}
	return creds, nil
}

// externalAgentWorkspace returns a codersdk.Workspace whose latest
// build has HasExternalAgent=true and one agent with the given name.
func externalAgentWorkspace(t *testing.T, name, agentName string) (codersdk.Workspace, uuid.UUID) {
	t.Helper()
	wsID := uuid.New()
	agentID := uuid.New()
	hasExternal := true
	return codersdk.Workspace{
		ID:   wsID,
		Name: name,
		LatestBuild: codersdk.WorkspaceBuild{
			HasExternalAgent: &hasExternal,
			Resources: []codersdk.WorkspaceResource{{
				Name: "external",
				Type: "coder_external_agent",
				Agents: []codersdk.WorkspaceAgent{{
					ID:   agentID,
					Name: agentName,
				}},
			}},
		},
	}, agentID
}

// Asserts the TokenInfo shape (workspace IDs, agent names, tokens)
// returned by the enumeration loop given a fake client.
func Test_Manager_EnumerateExternalAgents_returnsAllTokens(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	const numWorkspaces = 3
	workspaces := make([]codersdk.Workspace, 0, numWorkspaces)
	credentials := map[string]codersdk.ExternalAgentCredentials{}
	want := make([]agentfake.TokenInfo, 0, numWorkspaces)
	for i := 0; i < numWorkspaces; i++ {
		agentName := "external"
		ws, agentID := externalAgentWorkspace(t, "ws-"+uuid.NewString(), agentName)
		workspaces = append(workspaces, ws)
		token := uuid.NewString()
		credentials[ws.ID.String()+"/"+agentName] = codersdk.ExternalAgentCredentials{
			AgentToken: token,
		}
		want = append(want, agentfake.TokenInfo{
			WorkspaceID:   ws.ID,
			WorkspaceName: ws.Name,
			AgentID:       agentID,
			AgentName:     agentName,
			Token:         token,
		})
	}

	client := &fakeExternalAgentClient{workspaces: workspaces, credentials: credentials}
	coderURL, _ := url.Parse("http://fake")
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	m := agentfake.NewManager(coderURL, client, logger, agentfake.ManagerOptions{Template: "tmpl"})

	got, err := m.EnumerateExternalAgents(ctx)
	require.NoError(t, err)

	sortTokenInfosByWorkspaceID(want)
	sortTokenInfosByWorkspaceID(got)

	require.Equal(t, len(want), len(got), "expected one TokenInfo per external-agent workspace")
	for i := range want {
		assert.Equal(t, want[i].WorkspaceID, got[i].WorkspaceID, "WorkspaceID for entry %d", i)
		assert.Equal(t, want[i].AgentName, got[i].AgentName, "AgentName for entry %d", i)
		assert.Equal(t, want[i].Token, got[i].Token, "Token for entry %d", i)
		assert.NotEmpty(t, got[i].Token, "Token must be non-empty for entry %d", i)
	}
}

// Asserts that an authentication failure during enumeration produces a
// fatal error, so the retry loop in enumerateWithRetry surfaces it
// immediately rather than hammering endpoints with credentials that
// will never work.
func Test_Manager_EnumerateExternalAgents_invalidTokenIsFatal(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	client := &fakeExternalAgentClient{
		workspacesErr: codersdk.NewError(http.StatusUnauthorized, codersdk.Response{Message: "unauthorized"}),
	}
	coderURL, _ := url.Parse("http://fake")
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	m := agentfake.NewManager(coderURL, client, logger, agentfake.ManagerOptions{Template: "tmpl"})

	_, err := m.EnumerateExternalAgents(ctx)
	require.Error(t, err, "expected enumeration to fail with an invalid session token")
	require.True(t, agentfake.IsFatalEnumerationError(err),
		"expected error to be classified as fatal; got: %v", err)
}

func sortTokenInfosByWorkspaceID(s []agentfake.TokenInfo) {
	sort.Slice(s, func(i, j int) bool {
		return s[i].WorkspaceID.String() < s[j].WorkspaceID.String()
	})
}
