package coderd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestOrganizationSkillACL(t *testing.T) {
	t.Parallel()

	mAudit := audit.NewMockWithDiffFn(func(old, newVal any) audit.Map {
		oldSkill, oldOK := old.(database.AuditableOrganizationSkill)
		newSkill, newOK := newVal.(database.AuditableOrganizationSkill)
		if !oldOK || !newOK {
			return audit.Map{}
		}
		return audit.Map{
			"group_acl": {Old: oldSkill.GroupACL, New: newSkill.GroupACL},
			"user_acl":  {Old: oldSkill.UserACL, New: newSkill.UserACL},
		}
	})
	db, ps := dbtestutil.NewDB(t)
	ownerRawClient := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps, Auditor: mAudit})
	firstUser := coderdtest.CreateFirstUser(t, ownerRawClient)
	orgID := firstUser.OrganizationID
	owner := codersdk.NewExperimentalClient(ownerRawClient)
	orgAdminRawClient, _ := coderdtest.CreateAnotherUser(t, ownerRawClient, orgID, rbac.ScopedRoleOrgAdmin(orgID))
	memberRawClient, member := coderdtest.CreateAnotherUser(t, ownerRawClient, orgID)
	memberClient := codersdk.NewExperimentalClient(memberRawClient)
	auditorRawClient, _ := coderdtest.CreateAnotherUser(t, ownerRawClient, orgID, rbac.ScopedRoleOrgAuditor(orgID))

	otherOrg := dbgen.Organization(t, db, database.Organization{})
	foreignUser := dbgen.User(t, db, database.User{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: otherOrg.ID, UserID: foreignUser.ID})
	foreignGroup := dbgen.Group(t, db, database.Group{OrganizationID: otherOrg.ID})

	readACL := func(ids ...uuid.UUID) database.ChatACL {
		entries := database.ChatACL{}
		for _, id := range ids {
			entries[id.String()] = database.ChatACLEntry{Permissions: []policy.Action{policy.ActionRead}}
		}
		return entries
	}
	createSkill := func(ctx context.Context, t *testing.T, name string) codersdk.Skill {
		t.Helper()
		skill, err := owner.CreateOrganizationSkill(ctx, orgID, codersdk.CreateSkillRequest{
			Content: userSkillMarkdown(name, "ACL", "Body."),
		})
		require.NoError(t, err)
		return skill
	}

	t.Run("SharersManageACL", func(t *testing.T) {
		t.Parallel()

		for name, client := range map[string]*codersdk.ExperimentalClient{
			"org-admin":  codersdk.NewExperimentalClient(orgAdminRawClient),
			"site-owner": owner,
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitMedium)
				skill := createSkill(ctx, t, "sharer-"+name)

				acl, err := client.OrganizationSkillACL(ctx, orgID, skill.Name)
				require.NoError(t, err)
				require.Empty(t, acl.Users)
				require.Len(t, acl.Groups, 1)
				require.Equal(t, orgID, acl.Groups[0].ID)
				require.Equal(t, codersdk.OrganizationSkillRoleRead, acl.Groups[0].Role)

				available, err := client.OrganizationSkillACLAvailable(ctx, orgID, skill.Name, codersdk.UsersRequest{})
				require.NoError(t, err)
				require.Contains(t, aclAvailableUserIDs(available), member.ID)

				err = client.UpdateOrganizationSkillACL(ctx, orgID, skill.Name, codersdk.UpdateOrganizationSkillACLRequest{
					UserRoles: map[string]codersdk.OrganizationSkillRole{member.ID.String(): codersdk.OrganizationSkillRoleRead},
				})
				require.NoError(t, err)
				acl, err = client.OrganizationSkillACL(ctx, orgID, skill.Name)
				require.NoError(t, err)
				require.Len(t, acl.Users, 1)
				require.Equal(t, member.ID, acl.Users[0].ID)
				require.Equal(t, codersdk.OrganizationSkillRoleRead, acl.Users[0].Role)
			})
		}
	})

	t.Run("NonSharersGetNotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		skill := createSkill(ctx, t, "non-sharer-skill")
		for name, client := range map[string]*codersdk.ExperimentalClient{
			"EveryoneReader": memberClient,
			"OrgAuditor":     codersdk.NewExperimentalClient(auditorRawClient),
		} {
			_, err := client.OrganizationSkillByName(ctx, orgID, skill.Name)
			require.NoError(t, err, name)

			_, err = client.OrganizationSkillACL(ctx, orgID, skill.Name)
			requireSDKErrorStatus(t, err, http.StatusNotFound, name)
			err = client.UpdateOrganizationSkillACL(ctx, orgID, skill.Name, codersdk.UpdateOrganizationSkillACLRequest{
				UserRoles: map[string]codersdk.OrganizationSkillRole{member.ID.String(): codersdk.OrganizationSkillRoleRead},
			})
			requireSDKErrorStatus(t, err, http.StatusNotFound, name)
			_, err = client.OrganizationSkillACLAvailable(ctx, orgID, skill.Name, codersdk.UsersRequest{})
			requireSDKErrorStatus(t, err, http.StatusNotFound, name)
		}
	})

	t.Run("SparseUpdateChangesReaders", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		group := dbgen.Group(t, db, database.Group{OrganizationID: orgID})
		groupMemberRawClient, groupMember := coderdtest.CreateAnotherUser(t, ownerRawClient, orgID)
		dbgen.GroupMember(t, db, database.GroupMemberTable{GroupID: group.ID, UserID: groupMember.ID})
		addedRawClient, added := coderdtest.CreateAnotherUser(t, ownerRawClient, orgID)
		skill := createSkill(ctx, t, "sparse-acl-skill")
		requireOrganizationSkillListed(ctx, t, memberClient, orgID, skill.Name)

		err := owner.UpdateOrganizationSkillACL(ctx, orgID, skill.Name, codersdk.UpdateOrganizationSkillACLRequest{
			UserRoles: map[string]codersdk.OrganizationSkillRole{added.ID.String(): codersdk.OrganizationSkillRoleRead},
			GroupRoles: map[string]codersdk.OrganizationSkillRole{
				group.ID.String(): codersdk.OrganizationSkillRoleRead,
				orgID.String():    codersdk.OrganizationSkillRoleDeleted,
			},
		})
		require.NoError(t, err)

		requireOrganizationSkillNotListed(ctx, t, memberClient, orgID, skill.Name)
		_, err = memberClient.OrganizationSkillByName(ctx, orgID, skill.Name)
		requireSDKErrorStatus(t, err, http.StatusNotFound)
		for _, rawClient := range []*codersdk.Client{groupMemberRawClient, addedRawClient} {
			client := codersdk.NewExperimentalClient(rawClient)
			requireOrganizationSkillListed(ctx, t, client, orgID, skill.Name)
			_, err = client.OrganizationSkillByName(ctx, orgID, skill.Name)
			require.NoError(t, err)
		}

		var log database.AuditLog
		for _, entry := range mAudit.AuditLogs() {
			if entry.ResourceID == skill.ID && entry.Action == database.AuditActionWrite {
				log = entry
			}
		}
		require.Equal(t, database.ResourceTypeOrganizationSkill, log.ResourceType)
		require.Equal(t, orgID, log.OrganizationID)
		var diff map[string]struct {
			Old database.ChatACL `json:"old"`
			New database.ChatACL `json:"new"`
		}
		require.NoError(t, json.Unmarshal(log.Diff, &diff))
		require.Equal(t, readACL(orgID), diff["group_acl"].Old)
		require.Equal(t, readACL(group.ID), diff["group_acl"].New)
		require.Equal(t, database.ChatACL{}, diff["user_acl"].Old)
		require.Equal(t, readACL(added.ID), diff["user_acl"].New)

		// Removing principals that are no longer granted, or no longer exist,
		// leaves the ACL unchanged.
		err = owner.UpdateOrganizationSkillACL(ctx, orgID, skill.Name, codersdk.UpdateOrganizationSkillACLRequest{
			UserRoles:  map[string]codersdk.OrganizationSkillRole{uuid.NewString(): codersdk.OrganizationSkillRoleDeleted},
			GroupRoles: map[string]codersdk.OrganizationSkillRole{orgID.String(): codersdk.OrganizationSkillRoleDeleted},
		})
		require.NoError(t, err)
		acl, err := owner.OrganizationSkillACL(ctx, orgID, skill.Name)
		require.NoError(t, err)
		require.Len(t, acl.Users, 1)
		require.Equal(t, added.ID, acl.Users[0].ID)
		require.Len(t, acl.Groups, 1)
		require.Equal(t, group.ID, acl.Groups[0].ID)
		require.Equal(t, 1, acl.Groups[0].TotalMemberCount)
	})

	t.Run("RejectsInvalidUpdates", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		skill := createSkill(ctx, t, "invalid-acl-skill")
		for name, tc := range map[string]struct {
			req    codersdk.UpdateOrganizationSkillACLRequest
			detail string
		}{
			"UserOutsideOrganization": {
				req: codersdk.UpdateOrganizationSkillACLRequest{
					UserRoles: map[string]codersdk.OrganizationSkillRole{foreignUser.ID.String(): codersdk.OrganizationSkillRoleRead},
				},
				detail: "user " + foreignUser.ID.String() + " does not belong to organization",
			},
			"GroupFromOtherOrganization": {
				req: codersdk.UpdateOrganizationSkillACLRequest{
					GroupRoles: map[string]codersdk.OrganizationSkillRole{foreignGroup.ID.String(): codersdk.OrganizationSkillRoleRead},
				},
				detail: "group " + foreignGroup.ID.String() + " does not belong to organization",
			},
			"UnsupportedRole": {
				req: codersdk.UpdateOrganizationSkillACLRequest{
					UserRoles: map[string]codersdk.OrganizationSkillRole{member.ID.String(): "use"},
				},
				detail: `role "use" is not a valid organization skill role`,
			},
			"DuplicateSpellings": {
				req: codersdk.UpdateOrganizationSkillACLRequest{
					UserRoles: map[string]codersdk.OrganizationSkillRole{
						member.ID.String():                  codersdk.OrganizationSkillRoleRead,
						strings.ToUpper(member.ID.String()): codersdk.OrganizationSkillRoleDeleted,
					},
				},
				detail: "duplicate entries for ID " + member.ID.String(),
			},
		} {
			err := owner.UpdateOrganizationSkillACL(ctx, orgID, skill.Name, tc.req)
			sdkErr := requireSDKErrorStatus(t, err, http.StatusBadRequest, name)
			require.Contains(t, sdkErr.Error(), tc.detail, name)
		}

		acl, err := owner.OrganizationSkillACL(ctx, orgID, skill.Name)
		require.NoError(t, err)
		require.Empty(t, acl.Users)
		require.Len(t, acl.Groups, 1)
		require.Equal(t, orgID, acl.Groups[0].ID)
	})

	t.Run("AvailableListsOrganizationPrincipals", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		group := dbgen.Group(t, db, database.Group{OrganizationID: orgID})
		skill := createSkill(ctx, t, "available-acl-skill")

		available, err := owner.OrganizationSkillACLAvailable(ctx, orgID, skill.Name, codersdk.UsersRequest{})
		require.NoError(t, err)
		userIDs := aclAvailableUserIDs(available)
		require.Contains(t, userIDs, member.ID)
		require.NotContains(t, userIDs, foreignUser.ID)
		groupIDs := make([]uuid.UUID, 0, len(available.Groups))
		for _, availableGroup := range available.Groups {
			groupIDs = append(groupIDs, availableGroup.ID)
		}
		require.Contains(t, groupIDs, orgID)
		require.Contains(t, groupIDs, group.ID)
		require.NotContains(t, groupIDs, foreignGroup.ID)
	})

	t.Run("ScopedAPIKeys", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		skill := createSkill(ctx, t, "scoped-acl-skill")
		scopedClient := func(scopes ...database.APIKeyScope) *codersdk.ExperimentalClient {
			_, token := dbgen.APIKey(t, db, database.APIKey{UserID: firstUser.UserID, Scopes: scopes})
			rawClient := codersdk.New(ownerRawClient.URL)
			rawClient.SetSessionToken(token)
			return codersdk.NewExperimentalClient(rawClient)
		}
		req := codersdk.UpdateOrganizationSkillACLRequest{
			UserRoles: map[string]codersdk.OrganizationSkillRole{member.ID.String(): codersdk.OrganizationSkillRoleRead},
		}

		reader := scopedClient("organization:read", "organization_skill:read")
		_, err := reader.OrganizationSkillByName(ctx, orgID, skill.Name)
		require.NoError(t, err)
		err = reader.UpdateOrganizationSkillACL(ctx, orgID, skill.Name, req)
		requireSDKErrorStatus(t, err, http.StatusNotFound)

		sharer := scopedClient("organization:read", "organization_skill:share")
		require.NoError(t, sharer.UpdateOrganizationSkillACL(ctx, orgID, skill.Name, req))
	})
}

// organizationSkillDeleteRaceStore deletes the skill right after the param
// middleware read once armed, so the handler's locked re-fetch sees a
// concurrently deleted row.
type organizationSkillDeleteRaceStore struct {
	database.Store

	armed atomic.Bool
}

func (s *organizationSkillDeleteRaceStore) GetOrganizationSkillByOrganizationIDAndName(ctx context.Context, arg database.GetOrganizationSkillByOrganizationIDAndNameParams) (database.Skill, error) {
	skill, err := s.Store.GetOrganizationSkillByOrganizationIDAndName(ctx, arg)
	if err == nil && s.armed.CompareAndSwap(true, false) {
		if _, err := s.DeleteOrganizationSkillByOrganizationIDAndName(ctx, database.DeleteOrganizationSkillByOrganizationIDAndNameParams(arg)); err != nil {
			return database.Skill{}, err
		}
	}
	return skill, err
}

func TestOrganizationSkillACLConcurrentDelete(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	db, ps := dbtestutil.NewDB(t)
	store := &organizationSkillDeleteRaceStore{Store: db}
	ownerRawClient := coderdtest.New(t, &coderdtest.Options{Database: store, Pubsub: ps})
	firstUser := coderdtest.CreateFirstUser(t, ownerRawClient)
	client := codersdk.NewExperimentalClient(ownerRawClient)
	skill, err := client.CreateOrganizationSkill(ctx, firstUser.OrganizationID, codersdk.CreateSkillRequest{
		Content: userSkillMarkdown("acl-delete-race", "ACL", "Body."),
	})
	require.NoError(t, err)

	store.armed.Store(true)
	err = client.UpdateOrganizationSkillACL(ctx, firstUser.OrganizationID, skill.Name, codersdk.UpdateOrganizationSkillACLRequest{})
	requireSDKErrorStatus(t, err, http.StatusNotFound)
}

func aclAvailableUserIDs(available codersdk.ACLAvailable) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(available.Users))
	for _, user := range available.Users {
		ids = append(ids, user.ID)
	}
	return ids
}
