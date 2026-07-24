package agentfake_test

import (
	"context"
	"database/sql"
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
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/scaletest/agentfake"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

// fakeExternalAgentClient is an in-package fake for the ExternalAgentClient
// interface used by Manager to resolve names (template, owner) and to poll
// the workspace-count gate. The actual external-agent auth tokens are read
// from the real database.Store the tests seed via dbfake / dbgen.
//
// Tests populate me, owner, template, workspaces (the latter being a
// codersdk-shaped view of whichever rows the test seeded into the DB).
type fakeExternalAgentClient struct {
	me       codersdk.User
	owner    codersdk.User
	template codersdk.Template

	// workspaces, in the order Workspaces() should return them. Each call
	// returns up to filter.Limit entries starting at filter.Offset to model
	// pagination, matching real coderd behavior. Tests only need to populate
	// this when exercising the workspace-count gate; the new EnumerateExternalAgents
	// path doesn't list workspaces over HTTP at all.
	workspaces []codersdk.Workspace

	// meErr / templateErr are used by tests that want to verify resolution
	// errors are classified as fatal by the enumerate retry loop.
	meErr       error
	templateErr error
}

func (f *fakeExternalAgentClient) User(_ context.Context, userIdent string) (codersdk.User, error) {
	if userIdent == codersdk.Me {
		if f.meErr != nil {
			return codersdk.User{}, f.meErr
		}
		return f.me, nil
	}
	if userIdent == f.owner.Username {
		return f.owner, nil
	}
	return codersdk.User{}, xerrors.Errorf("no user %q", userIdent)
}

func (f *fakeExternalAgentClient) Template(_ context.Context, id uuid.UUID) (codersdk.Template, error) {
	if f.templateErr != nil {
		return codersdk.Template{}, f.templateErr
	}
	if id == f.template.ID {
		return f.template, nil
	}
	return codersdk.Template{}, xerrors.Errorf("no template with id %s", id)
}

func (f *fakeExternalAgentClient) TemplatesByOrganization(_ context.Context, orgID uuid.UUID) ([]codersdk.Template, error) {
	if f.templateErr != nil {
		return nil, f.templateErr
	}
	if f.template.ID == uuid.Nil || f.template.OrganizationID != orgID {
		return nil, nil
	}
	return []codersdk.Template{f.template}, nil
}

func (f *fakeExternalAgentClient) Workspaces(_ context.Context, filter codersdk.WorkspaceFilter) (codersdk.WorkspacesResponse, error) {
	start := filter.Offset
	if start > len(f.workspaces) {
		start = len(f.workspaces)
	}
	end := start + filter.Limit
	if filter.Limit == 0 || end > len(f.workspaces) {
		end = len(f.workspaces)
	}
	return codersdk.WorkspacesResponse{
		Workspaces: f.workspaces[start:end],
		Count:      len(f.workspaces),
	}, nil
}

// seedUserOrgAndTemplate sets up the minimum DB rows needed for a workspace's
// FK constraints to hold, and returns the IDs the caller will reuse when
// seeding workspaces and populating the fake client.
func seedUserOrgAndTemplate(t *testing.T, db database.Store) (org database.Organization, user database.User, tpl database.Template) {
	t.Helper()
	org = dbgen.Organization(t, db, database.Organization{})
	user = dbgen.User(t, db, database.User{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	tpl = dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		ActiveVersionID: tv.ID,
		CreatedBy:       user.ID,
	})
	return org, user, tpl
}

// buildExternalAgentWorkspace creates one workspace with a coder_external_agent
// resource, an agent, and HasExternalAgent=true on the latest build. The
// latest build's provisioner job is Succeeded by default (the dbfake default),
// which is what the "running" filter in GetExternalAgentTokensByTemplateID
// requires.
func buildExternalAgentWorkspace(
	t *testing.T,
	db database.Store,
	orgID, ownerID, templateID uuid.UUID,
) dbfake.WorkspaceResponse {
	t.Helper()
	return dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: orgID,
		OwnerID:        ownerID,
		TemplateID:     templateID,
	}).
		Seed(database.WorkspaceBuild{
			HasExternalAgent: sql.NullBool{Bool: true, Valid: true},
		}).
		Resource(&sdkproto.Resource{
			Name: "external",
			Type: "coder_external_agent",
		}).
		WithAgent().
		Do()
}

// newFakeClient builds a fakeExternalAgentClient consistent with the rows the
// caller seeded into the DB. me is the user that the manager will call
// User(codersdk.Me) on; its OrganizationIDs is what parseTemplate walks.
func newFakeClient(me database.User, org database.Organization, tpl database.Template) *fakeExternalAgentClient {
	return &fakeExternalAgentClient{
		me: codersdk.User{
			ReducedUser:     codersdk.ReducedUser{MinimalUser: codersdk.MinimalUser{ID: me.ID, Username: me.Username}},
			OrganizationIDs: []uuid.UUID{org.ID},
		},
		template: codersdk.Template{
			ID:             tpl.ID,
			OrganizationID: org.ID,
			Name:           tpl.Name,
		},
	}
}

// Asserts the TokenInfo shape (workspace IDs, agent names, tokens) returned by
// the enumeration loop reads from the DB the test seeded.
func Test_Manager_EnumerateExternalAgents_returnsAllTokens(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	db, _ := dbtestutil.NewDB(t)
	org, user, tpl := seedUserOrgAndTemplate(t, db)

	const numWorkspaces = 3
	want := make([]agentfake.TokenInfo, 0, numWorkspaces)
	for i := 0; i < numWorkspaces; i++ {
		r := buildExternalAgentWorkspace(t, db, org.ID, user.ID, tpl.ID)
		want = append(want, agentfake.TokenInfo{
			WorkspaceID:   r.Workspace.ID,
			WorkspaceName: r.Workspace.Name,
			AgentID:       r.Agents[0].ID,
			AgentName:     r.Agents[0].Name,
			Token:         r.AgentToken,
		})
	}

	client := newFakeClient(user, org, tpl)
	coderURL, _ := url.Parse("http://fake")
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	m := agentfake.NewManager(logger, coderURL, client, db, agentfake.ManagerOptions{Template: tpl.Name})
	require.NoError(t, m.ResolveTemplateAndOwner(ctx))

	got, err := m.EnumerateExternalAgents(ctx)
	require.NoError(t, err)

	sortTokenInfosByWorkspaceID(want)
	sortTokenInfosByWorkspaceID(got)

	require.Equal(t, len(want), len(got),
		"expected one TokenInfo per external-agent workspace under the template")
	for i := range want {
		assert.Equal(t, want[i].WorkspaceID, got[i].WorkspaceID, "WorkspaceID for entry %d", i)
		assert.Equal(t, want[i].WorkspaceName, got[i].WorkspaceName, "WorkspaceName for entry %d", i)
		assert.Equal(t, want[i].AgentName, got[i].AgentName, "AgentName for entry %d", i)
		assert.Equal(t, want[i].Token, got[i].Token, "Token for entry %d", i)
		assert.NotEmpty(t, got[i].Token, "Token must be non-empty for entry %d", i)
	}
}

// Asserts that an authentication failure surfaced during template/owner
// resolution is fatal, so Run does not retry indefinitely against credentials
// that will never work.
func Test_Manager_ResolveTemplateAndOwner_invalidTokenIsFatal(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	db, _ := dbtestutil.NewDB(t)
	client := &fakeExternalAgentClient{
		meErr: codersdk.NewError(http.StatusUnauthorized, codersdk.Response{Message: "unauthorized"}),
	}
	coderURL, _ := url.Parse("http://fake")
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	m := agentfake.NewManager(logger, coderURL, client, db, agentfake.ManagerOptions{Template: "tmpl"})

	err := m.ResolveTemplateAndOwner(ctx)
	require.Error(t, err, "expected resolution to fail with an invalid session token")
	require.True(t, agentfake.IsFatalEnumerationError(err),
		"expected error to be classified as fatal; got: %v", err)
}

// Asserts that --owner restricts results to workspaces owned by that user even
// when other owners have external-agent workspaces under the same template.
func Test_Manager_EnumerateExternalAgents_filtersByOwner(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	db, _ := dbtestutil.NewDB(t)
	org, firstUser, tpl := seedUserOrgAndTemplate(t, db)
	secondUser := dbgen.User(t, db, database.User{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         secondUser.ID,
		OrganizationID: org.ID,
	})

	_ = buildExternalAgentWorkspace(t, db, org.ID, firstUser.ID, tpl.ID)
	r2 := buildExternalAgentWorkspace(t, db, org.ID, secondUser.ID, tpl.ID)

	client := newFakeClient(firstUser, org, tpl)
	client.owner = codersdk.User{
		ReducedUser: codersdk.ReducedUser{MinimalUser: codersdk.MinimalUser{
			ID: secondUser.ID, Username: secondUser.Username,
		}},
		OrganizationIDs: []uuid.UUID{org.ID},
	}
	coderURL, _ := url.Parse("http://fake")
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	m := agentfake.NewManager(logger, coderURL, client, db, agentfake.ManagerOptions{
		Template: tpl.Name,
		Owner:    secondUser.Username,
	})
	require.NoError(t, m.ResolveTemplateAndOwner(ctx))

	got, err := m.EnumerateExternalAgents(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1, "expected only the second user's workspace to be returned")
	require.Equal(t, r2.Workspace.ID, got[0].WorkspaceID)
	require.Equal(t, r2.AgentToken, got[0].Token)
}

// Asserts that workspaces whose latest build is not in the "running" state
// (job_status != succeeded or transition != start) are excluded from
// enumeration results.
func Test_Manager_EnumerateExternalAgents_excludesNonRunning(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	db, _ := dbtestutil.NewDB(t)
	org, user, tpl := seedUserOrgAndTemplate(t, db)

	// Running workspace: should be included.
	running := buildExternalAgentWorkspace(t, db, org.ID, user.ID, tpl.ID)

	// Failed-build workspace under the same template: should be excluded.
	_ = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		TemplateID:     tpl.ID,
	}).
		Seed(database.WorkspaceBuild{
			HasExternalAgent: sql.NullBool{Bool: true, Valid: true},
		}).
		Resource(&sdkproto.Resource{
			Name: "external",
			Type: "coder_external_agent",
		}).
		WithAgent().
		Failed().
		Do()

	client := newFakeClient(user, org, tpl)
	coderURL, _ := url.Parse("http://fake")
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	m := agentfake.NewManager(logger, coderURL, client, db, agentfake.ManagerOptions{Template: tpl.Name})
	require.NoError(t, m.ResolveTemplateAndOwner(ctx))

	got, err := m.EnumerateExternalAgents(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1, "only the running workspace should be returned")
	require.Equal(t, running.Workspace.ID, got[0].WorkspaceID)
}

func sortTokenInfosByWorkspaceID(s []agentfake.TokenInfo) {
	sort.Slice(s, func(i, j int) bool {
		return s[i].WorkspaceID.String() < s[j].WorkspaceID.String()
	})
}
