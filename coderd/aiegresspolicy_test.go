package coderd_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestAIEgressPolicyDefault(t *testing.T) {
	t.Parallel()

	client, _, user, template := setupAIEgressPolicyTemplate(t, nil)
	ctx := testutil.Context(t, testutil.WaitLong)

	policy, err := client.TemplateAIEgressPolicy(ctx, template.ID)
	require.NoError(t, err)
	require.Equal(t, template.ID, policy.TemplateID)
	require.Zero(t, policy.Revision)
	require.Empty(t, policy.Rules)
	require.Equal(t, uuid.Nil, policy.UpdatedBy)
	require.NotEqual(t, uuid.Nil, user.UserID)
}

func TestAIEgressPolicyRoundTrip(t *testing.T) {
	t.Parallel()

	client, _, user, template := setupAIEgressPolicyTemplate(t, nil)
	ctx := testutil.Context(t, testutil.WaitLong)

	firstRules := []codersdk.AIEgressRule{{Host: "GitHub.COM", Ports: []int{443}}}
	first, err := client.UpdateTemplateAIEgressPolicy(ctx, template.ID, codersdk.UpdateAIEgressPolicyRequest{Rules: firstRules})
	require.NoError(t, err)
	require.EqualValues(t, 1, first.Revision)
	require.Equal(t, user.UserID, first.UpdatedBy)
	require.Equal(t, []codersdk.AIEgressRule{{Host: "github.com", Ports: []int{443}}}, first.Rules)

	secondRules := []codersdk.AIEgressRule{{Host: "*.Example.COM"}}
	second, err := client.UpdateTemplateAIEgressPolicy(ctx, template.ID, codersdk.UpdateAIEgressPolicyRequest{Rules: secondRules})
	require.NoError(t, err)
	require.EqualValues(t, 2, second.Revision)
	require.Equal(t, user.UserID, second.UpdatedBy)
	require.Equal(t, []codersdk.AIEgressRule{{Host: "*.example.com"}}, second.Rules)

	got, err := client.TemplateAIEgressPolicy(ctx, template.ID)
	require.NoError(t, err)
	require.Equal(t, second, got)
}

func TestAIEgressPolicyValidation(t *testing.T) {
	t.Parallel()

	tooManyRules := make([]codersdk.AIEgressRule, 129)
	for i := range tooManyRules {
		tooManyRules[i] = codersdk.AIEgressRule{Host: "example.com"}
	}

	tests := []struct {
		name  string
		rules []codersdk.AIEgressRule
	}{
		{name: "WildcardInMiddle", rules: []codersdk.AIEgressRule{{Host: "a.*.b"}}},
		{name: "PortZero", rules: []codersdk.AIEgressRule{{Host: "example.com", Ports: []int{0}}}},
		{name: "PortTooLarge", rules: []codersdk.AIEgressRule{{Host: "example.com", Ports: []int{70000}}}},
		{name: "Scheme", rules: []codersdk.AIEgressRule{{Host: "https://example.com"}}},
		{name: "TooManyRules", rules: tooManyRules},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, _, _, template := setupAIEgressPolicyTemplate(t, nil)
			ctx := testutil.Context(t, testutil.WaitLong)
			_, err := client.UpdateTemplateAIEgressPolicy(ctx, template.ID, codersdk.UpdateAIEgressPolicyRequest{Rules: tt.rules})
			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr)
			require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
			require.NotEmpty(t, sdkErr.Validations)

			policy, err := client.TemplateAIEgressPolicy(ctx, template.ID)
			require.NoError(t, err)
			require.Zero(t, policy.Revision)
		})
	}
}

func TestAIEgressPolicyRBAC(t *testing.T) {
	t.Parallel()

	ownerClient, _, owner, template := setupAIEgressPolicyTemplate(t, nil)
	templateAdmin, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.ScopedRoleOrgTemplateAdmin(owner.OrganizationID))
	member, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
	ctx := testutil.Context(t, testutil.WaitLong)

	adminPolicy, err := templateAdmin.UpdateTemplateAIEgressPolicy(ctx, template.ID, codersdk.UpdateAIEgressPolicyRequest{
		Rules: []codersdk.AIEgressRule{{Host: "admin.example.com", Ports: []int{443}}},
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, adminPolicy.Revision)

	_, err = member.UpdateTemplateAIEgressPolicy(ctx, template.ID, codersdk.UpdateAIEgressPolicyRequest{
		Rules: []codersdk.AIEgressRule{{Host: "member.example.com", Ports: []int{443}}},
	})
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Contains(t, []int{http.StatusForbidden, http.StatusNotFound}, sdkErr.StatusCode())

	got, err := templateAdmin.TemplateAIEgressPolicy(ctx, template.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, got.Revision)
	require.Equal(t, adminPolicy.Rules, got.Rules)
}

func TestAIEgressPolicyAudit(t *testing.T) {
	t.Parallel()

	auditor := audit.NewMock()
	client, _, _, template := setupAIEgressPolicyTemplate(t, &coderdtest.Options{Auditor: auditor})
	auditor.ResetLogs()
	ctx := testutil.Context(t, testutil.WaitLong)

	policy, err := client.UpdateTemplateAIEgressPolicy(ctx, template.ID, codersdk.UpdateAIEgressPolicyRequest{
		Rules: []codersdk.AIEgressRule{{Host: "AUDIT.EXAMPLE.COM", Ports: []int{8443}}},
	})
	require.NoError(t, err)

	var found database.AuditLog
	for _, log := range auditor.AuditLogs() {
		if log.Action == database.AuditActionWrite && log.ResourceType == database.ResourceTypeTemplate && log.ResourceID == template.ID {
			found = log
			break
		}
	}
	require.NotEqual(t, uuid.Nil, found.ID)
	require.EqualValues(t, http.StatusOK, found.StatusCode)

	var additional struct {
		OldRevision int64                   `json:"old_revision"`
		NewRevision int64                   `json:"new_revision"`
		OldRules    []codersdk.AIEgressRule `json:"old_rules"`
		NewRules    []codersdk.AIEgressRule `json:"new_rules"`
	}
	require.NoError(t, json.Unmarshal(found.AdditionalFields, &additional))
	require.Zero(t, additional.OldRevision)
	require.Equal(t, policy.Revision, additional.NewRevision)
	require.Empty(t, additional.OldRules)
	require.Equal(t, policy.Rules, additional.NewRules)
}

func TestAIEgressPolicyInvalidStoredRules(t *testing.T) {
	t.Parallel()

	client, db, user, template := setupAIEgressPolicyTemplate(t, nil)
	ctx := testutil.Context(t, testutil.WaitLong)
	owner, err := client.User(ctx, codersdk.Me)
	require.NoError(t, err)
	_, err = db.InsertTemplateAIEgressPolicy(dbauthz.As(ctx, coderdtest.AuthzUserSubject(owner)), database.InsertTemplateAIEgressPolicyParams{
		TemplateID: template.ID,
		Rules:      json.RawMessage(`{}`),
		CreatedBy:  user.UserID,
	})
	require.NoError(t, err)

	_, err = client.TemplateAIEgressPolicy(ctx, template.ID)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusInternalServerError, sdkErr.StatusCode())
}

func TestAIEgressPolicyAgentGet(t *testing.T) {
	t.Parallel()

	accessURL, err := url.Parse("https://coder.example.com")
	require.NoError(t, err)
	client, db, user, template := setupAIEgressPolicyTemplate(t, &coderdtest.Options{AccessURL: accessURL})
	ctx := testutil.Context(t, testutil.WaitLong)

	stored, err := client.UpdateTemplateAIEgressPolicy(ctx, template.ID, codersdk.UpdateAIEgressPolicyRequest{
		Rules: []codersdk.AIEgressRule{{Host: "packages.example.com", Ports: []int{443}}},
	})
	require.NoError(t, err)

	workspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
		TemplateID:     template.ID,
	}).WithAgent().Do()
	agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(workspace.AgentToken))

	policy, err := agentClient.AIEgressPolicy(ctx)
	require.NoError(t, err)
	require.Equal(t, stored.Revision, policy.Revision)
	require.Equal(t, stored.UpdatedBy, policy.UpdatedBy)
	require.Equal(t, []codersdk.AIEgressRule{
		{Host: "packages.example.com", Ports: []int{443}},
		{Host: "coder.example.com", Ports: []int{443}},
	}, policy.Rules)
}

func TestAIEgressPolicyWatch(t *testing.T) {
	t.Parallel()

	client, db, user, template := setupAIEgressPolicyTemplate(t, nil)
	workspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
		TemplateID:     template.ID,
	}).WithAgent().Do()
	agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(workspace.AgentToken))
	ctx := testutil.Context(t, testutil.WaitLong)

	policies, closer, err := agentClient.WatchAIEgressPolicy(ctx)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, closer.Close())
	}()

	initial := testutil.RequireReceive(ctx, t, policies)
	require.Zero(t, initial.Revision)
	require.Len(t, initial.Rules, 1)

	stored, err := client.UpdateTemplateAIEgressPolicy(ctx, template.ID, codersdk.UpdateAIEgressPolicyRequest{
		Rules: []codersdk.AIEgressRule{{Host: "updates.example.com", Ports: []int{443}}},
	})
	require.NoError(t, err)

	updated := testutil.RequireReceive(ctx, t, policies)
	require.Equal(t, stored.Revision, updated.Revision)
	require.Len(t, updated.Rules, len(stored.Rules)+1)
	require.Equal(t, stored.Rules, updated.Rules[:len(stored.Rules)])
}

func setupAIEgressPolicyTemplate(t *testing.T, options *coderdtest.Options) (*codersdk.Client, database.Store, codersdk.CreateFirstUserResponse, codersdk.Template) {
	t.Helper()

	client, db := coderdtest.NewWithDatabase(t, options)
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	return client, db, user, template
}
