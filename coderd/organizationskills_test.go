package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestOrganizationSkillAccess(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ownerClient := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps})
	firstUser := coderdtest.CreateFirstUser(t, ownerClient)
	orgID := firstUser.OrganizationID
	admin := codersdk.NewExperimentalClient(ownerClient)
	memberRawClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, orgID)
	member := codersdk.NewExperimentalClient(memberRawClient)
	sharedRawClient, sharedUser := coderdtest.CreateAnotherUser(t, ownerClient, orgID)
	shared := codersdk.NewExperimentalClient(sharedRawClient)
	auditorRawClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, orgID, rbac.ScopedRoleOrgAuditor(orgID))
	auditor := codersdk.NewExperimentalClient(auditorRawClient)

	everyoneACL := func() database.ChatACL {
		return database.ChatACL{orgID.String(): {Permissions: []policy.Action{policy.ActionRead}}}
	}
	everyone := dbgen.OrganizationSkill(t, db, database.Skill{
		OrganizationID: uuid.NullUUID{UUID: orgID, Valid: true},
		Name:           "everyone-skill",
		GroupACL:       everyoneACL(),
		UserACL:        database.ChatACL{},
	})
	userShared := dbgen.OrganizationSkill(t, db, database.Skill{
		OrganizationID: uuid.NullUUID{UUID: orgID, Valid: true},
		Name:           "user-shared-skill",
		GroupACL:       database.ChatACL{},
		UserACL: database.ChatACL{
			sharedUser.ID.String(): {Permissions: []policy.Action{policy.ActionRead}},
		},
	})

	t.Run("MemberReadsThroughEveryone", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		requireOrganizationSkillListed(ctx, t, member, orgID, everyone.Name)
		_, err := member.OrganizationSkillByName(ctx, orgID, everyone.Name)
		require.NoError(t, err)
	})

	t.Run("UserACLGrantsOnlyThatUser", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		requireOrganizationSkillListed(ctx, t, shared, orgID, userShared.Name)
		_, err := shared.OrganizationSkillByName(ctx, orgID, userShared.Name)
		require.NoError(t, err)

		requireOrganizationSkillNotListed(ctx, t, member, orgID, userShared.Name)
		_, err = member.OrganizationSkillByName(ctx, orgID, userShared.Name)
		requireSDKErrorStatus(t, err, http.StatusNotFound)
	})

	t.Run("OrgAuditorReads", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		requireOrganizationSkillListed(ctx, t, auditor, orgID, userShared.Name)
		_, err := auditor.OrganizationSkillByName(ctx, orgID, userShared.Name)
		require.NoError(t, err)
	})

	t.Run("OtherOrganizationMember", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		otherOrg := dbgen.Organization(t, db, database.Organization{})
		foreignUser := dbgen.User(t, db, database.User{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: otherOrg.ID, UserID: foreignUser.ID})
		_, token := dbgen.APIKey(t, db, database.APIKey{UserID: foreignUser.ID})
		foreignRawClient := codersdk.New(ownerClient.URL)
		foreignRawClient.SetSessionToken(token)
		foreign := codersdk.NewExperimentalClient(foreignRawClient)

		_, err := foreign.OrganizationSkills(ctx, orgID)
		requireSDKErrorStatus(t, err, http.StatusNotFound)
		_, err = foreign.OrganizationSkillByName(ctx, orgID, everyone.Name)
		requireSDKErrorStatus(t, err, http.StatusNotFound)
	})

	t.Run("ReadScopedAPIKey", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		_, token := dbgen.APIKey(t, db, database.APIKey{
			UserID: firstUser.UserID,
			Scopes: database.APIKeyScopes{"organization:read", "organization_skill:read"},
		})
		scopedRawClient := codersdk.New(ownerClient.URL)
		scopedRawClient.SetSessionToken(token)
		scoped := codersdk.NewExperimentalClient(scopedRawClient)

		requireOrganizationSkillListed(ctx, t, scoped, orgID, everyone.Name)
		_, err := scoped.OrganizationSkillByName(ctx, orgID, everyone.Name)
		require.NoError(t, err)
	})
}

func requireOrganizationSkillListed(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, orgID uuid.UUID, name string) codersdk.SkillMetadata {
	t.Helper()
	list, err := client.OrganizationSkills(ctx, orgID)
	require.NoError(t, err)
	for _, skill := range list {
		if skill.Name == name {
			return skill
		}
	}
	require.Failf(t, "organization skill not listed", "%q missing from %v", name, list)
	return codersdk.SkillMetadata{}
}

func requireOrganizationSkillNotListed(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, orgID uuid.UUID, name string) {
	t.Helper()
	list, err := client.OrganizationSkills(ctx, orgID)
	require.NoError(t, err)
	for _, skill := range list {
		require.NotEqual(t, name, skill.Name, "organization skill unexpectedly listed")
	}
}
