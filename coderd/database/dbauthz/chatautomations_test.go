package dbauthz_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/testutil"
)

// TestChatAutomationAuthorization runs the chat automation queries through
// dbauthz with the real authorizer, real roles and a real database. Each
// subject is loaded from its user row and organization membership, the way
// the API key middleware loads it.
func TestChatAutomationAuthorization(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	rawDB, _ := dbtestutil.NewDB(t)
	authorizer := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())
	db := dbauthz.New(rawDB, authorizer, slog.Make(), coderdtest.AccessControlStorePointer())

	org := dbgen.Organization(t, rawDB, database.Organization{})
	otherOrg := dbgen.Organization(t, rawDB, database.Organization{})
	// Every member gets the default roles of the organization, and
	// agents-access is one of them. This organization took it out.
	noAgentsOrg := dbgen.Organization(t, rawDB, database.Organization{
		DefaultOrgMemberRoles: []string{rbac.RoleOrgWorkspaceAccess()},
	})
	newMember := func(orgID uuid.UUID, siteRoles []string, orgRoles ...string) database.User {
		user := dbgen.User(t, rawDB, database.User{RBACRoles: siteRoles})
		dbgen.OrganizationMember(t, rawDB, database.OrganizationMember{
			UserID: user.ID, OrganizationID: orgID, Roles: orgRoles,
		})
		return user
	}
	member := newMember(org.ID, nil)
	otherMember := newMember(org.ID, nil)
	orgAdmin := newMember(org.ID, nil, rbac.RoleOrgAdmin())
	otherOrgAdmin := newMember(otherOrg.ID, nil, rbac.RoleOrgAdmin())
	deploymentOwner := newMember(org.ID, []string{rbac.RoleOwner().String()})
	memberWithoutAgentsAccess := newMember(noAgentsOrg.ID, nil)
	auditorWithoutAgentsAccess := newMember(noAgentsOrg.ID, []string{rbac.RoleAuditor().String()})

	// A custom role that grants every chat automation action for the whole
	// organization, create included.
	customRole, err := rawDB.InsertCustomRole(context.Background(), database.InsertCustomRoleParams{
		Name:            "automation-manager",
		OrganizationID:  uuid.NullUUID{UUID: org.ID, Valid: true},
		SitePermissions: database.CustomRolePermissions{},
		OrgPermissions: database.CustomRolePermissions{
			{ResourceType: rbac.ResourceChatAutomation.Type, Action: policy.WildcardSymbol},
			{ResourceType: rbac.ResourceChat.Type, Action: policy.ActionUpdate},
			{ResourceType: rbac.ResourceChat.Type, Action: policy.ActionRead},
		},
		UserPermissions:   database.CustomRolePermissions{},
		MemberPermissions: database.CustomRolePermissions{},
	})
	require.NoError(t, err)
	customManager := newMember(org.ID, nil, customRole.Name)

	as := func(t *testing.T, user database.User, scope rbac.ExpandableScope) context.Context {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)
		subject, _, err := httpmw.UserRBACSubject(ctx, rawDB, user.ID, scope)
		require.NoError(t, err)
		return dbauthz.As(ctx, subject)
	}
	asUser := func(t *testing.T, user database.User) context.Context {
		t.Helper()
		return as(t, user, rbac.ScopeAll)
	}

	model := dbgen.ChatModelConfig(t, rawDB, database.ChatModelConfig{OrganizationID: org.ID})
	newChat := func(owner database.User) database.Chat {
		return dbgen.Chat(t, rawDB, database.Chat{OrganizationID: org.ID, OwnerID: owner.ID, LastModelConfigID: model.ID})
	}
	memberChat := newChat(member)
	otherMemberChat := newChat(otherMember)
	memberProject := dbgen.ChatProject(t, rawDB, database.ChatProject{OrganizationID: org.ID, OwnerID: member.ID})
	otherMemberProject := dbgen.ChatProject(t, rawDB, database.ChatProject{OrganizationID: org.ID, OwnerID: otherMember.ID})
	// A model that only otherMember may use.
	privateModel := dbgen.ChatModelConfig(t, rawDB, database.ChatModelConfig{
		OrganizationID: org.ID,
		GroupACL:       database.ChatACL{},
		UserACL:        database.ChatACL{otherMember.ID.String(): {Permissions: []policy.Action{policy.ActionRead}}},
	})

	webhookIn := func(orgID uuid.UUID, owner database.User) database.InsertChatAutomationParams {
		return database.InsertChatAutomationParams{
			OrganizationID: orgID,
			OwnerID:        owner.ID,
			Name:           "webhook",
			Kind:           database.ChatAutomationKindWebhook,
			Prompt:         "Investigate the event.",
			TargetMode:     database.ChatAutomationTargetModeExistingChat,
			WhenBusy:       database.NullChatAutomationWhenBusy{ChatAutomationWhenBusy: database.ChatAutomationWhenBusyQueue, Valid: true},
			WebhookAuth:    database.NullChatAutomationWebhookAuth{ChatAutomationWebhookAuth: database.ChatAutomationWebhookAuthBearer, Valid: true},
			WebhookSecret:  sql.NullString{String: "webhook-secret", Valid: true},
		}
	}
	webhook := func(owner database.User) database.InsertChatAutomationParams {
		return webhookIn(org.ID, owner)
	}
	schedule := func(owner database.User, modelID uuid.UUID) database.InsertChatAutomationParams {
		return database.InsertChatAutomationParams{
			OrganizationID:       org.ID,
			OwnerID:              owner.ID,
			Name:                 "schedule",
			Kind:                 database.ChatAutomationKindSchedule,
			Prompt:               "Post the daily report.",
			TargetMode:           database.ChatAutomationTargetModeNewChat,
			NewChatModelConfigID: uuid.NullUUID{UUID: modelID, Valid: true},
			ScheduleCron:         sql.NullString{String: "0 9 * * *", Valid: true},
			ScheduleTimeZone:     sql.NullString{String: "UTC", Valid: true},
		}
	}
	requireDenied := func(t *testing.T, err error) {
		t.Helper()
		require.Error(t, err)
		require.True(t, dbauthz.IsNotAuthorizedError(err), "want a not authorized error, got %v", err)
	}

	t.Run("Insert", func(t *testing.T) {
		t.Parallel()

		nullID := func(id uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: id, Valid: true} }
		for _, tc := range []struct {
			name    string
			actor   database.User
			params  database.InsertChatAutomationParams
			allowed bool
		}{
			{name: "MemberOwnWithOwnReferences", actor: member, allowed: true, params: func() database.InsertChatAutomationParams {
				p := webhook(member)
				p.TargetChatID, p.CreatedByChatID = nullID(memberChat.ID), nullID(memberChat.ID)
				return p
			}()},
			{name: "MemberOwnNewChatWithOwnProject", actor: member, allowed: true, params: func() database.InsertChatAutomationParams {
				p := schedule(member, model.ID)
				p.ProjectID = nullID(memberProject.ID)
				return p
			}()},
			// Administrators create their own automations as members.
			{name: "OrgAdminOwn", actor: orgAdmin, allowed: true, params: webhook(orgAdmin)},
			{name: "DeploymentOwnerOwn", actor: deploymentOwner, allowed: true, params: webhook(deploymentOwner)},
			// Nobody creates an automation that runs as another user.
			{name: "MemberForAnotherOwner", actor: member, params: webhook(otherMember)},
			{name: "OrgAdminForAnotherOwner", actor: orgAdmin, params: webhook(member)},
			{name: "DeploymentOwnerForAnotherOwner", actor: deploymentOwner, params: webhook(member)},
			// The custom role grants create for the whole organization and
			// update on every chat. The owner check still refuses.
			{name: "CustomRoleForAnotherOwner", actor: customManager, params: func() database.InsertChatAutomationParams {
				p := webhook(member)
				p.TargetChatID = nullID(memberChat.ID)
				return p
			}()},
			{name: "CustomRoleOwn", actor: customManager, allowed: true, params: func() database.InsertChatAutomationParams {
				p := webhook(customManager)
				p.TargetChatID = nullID(memberChat.ID)
				return p
			}()},
			{name: "MemberWithoutAgentsAccess", actor: memberWithoutAgentsAccess, params: webhookIn(noAgentsOrg.ID, memberWithoutAgentsAccess)},
			{name: "AuditorWithoutAgentsAccess", actor: auditorWithoutAgentsAccess, params: webhookIn(noAgentsOrg.ID, auditorWithoutAgentsAccess)},
			// Each reference is checked on its own.
			{name: "TargetChatOfAnotherMember", actor: member, params: func() database.InsertChatAutomationParams {
				p := webhook(member)
				p.TargetChatID = nullID(otherMemberChat.ID)
				return p
			}()},
			{name: "CreatingChatOfAnotherMember", actor: member, params: func() database.InsertChatAutomationParams {
				p := webhook(member)
				p.CreatedByChatID = nullID(otherMemberChat.ID)
				return p
			}()},
			{name: "ModelWithoutAccess", actor: member, params: schedule(member, privateModel.ID)},
			{name: "ProjectOfAnotherMember", actor: member, params: func() database.InsertChatAutomationParams {
				p := schedule(member, model.ID)
				p.ProjectID = nullID(otherMemberProject.ID)
				return p
			}()},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				automation, err := db.InsertChatAutomation(asUser(t, tc.actor), tc.params)
				if !tc.allowed {
					requireDenied(t, err)
					return
				}
				require.NoError(t, err)
				require.Equal(t, tc.actor.ID, automation.OwnerID)
			})
		}
	})

	// Rows for the read, list and delete cases. dbgen inserts each one as
	// its owner.
	memberAutomation := dbgen.ChatAutomation(t, db, database.ChatAutomation{OrganizationID: org.ID, OwnerID: member.ID, Name: "member"})
	otherMemberAutomation := dbgen.ChatAutomation(t, db, database.ChatAutomation{OrganizationID: org.ID, OwnerID: otherMember.ID, Name: "other-member"})
	// An automation whose owner has no agents-access. The owner can no
	// longer reach it.
	orphanedAutomation := dbgen.ChatAutomation(t, db, database.ChatAutomation{OrganizationID: noAgentsOrg.ID, OwnerID: memberWithoutAgentsAccess.ID})

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name       string
			actor      database.User
			automation database.ChatAutomation
			allowed    bool
		}{
			{name: "Owner", actor: member, automation: memberAutomation, allowed: true},
			{name: "AnotherMember", actor: otherMember, automation: memberAutomation},
			{name: "OwnerWithoutAgentsAccess", actor: memberWithoutAgentsAccess, automation: orphanedAutomation},
			{name: "AuditorWithoutAgentsAccess", actor: auditorWithoutAgentsAccess, automation: orphanedAutomation},
			{name: "OrgAdmin", actor: orgAdmin, automation: memberAutomation, allowed: true},
			{name: "OrgAdminOfAnotherOrganization", actor: otherOrgAdmin, automation: memberAutomation},
			{name: "DeploymentOwner", actor: deploymentOwner, automation: memberAutomation, allowed: true},
			{name: "DeploymentOwnerInAnotherOrganization", actor: deploymentOwner, automation: orphanedAutomation, allowed: true},
			{name: "CustomRole", actor: customManager, automation: memberAutomation, allowed: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				got, err := db.GetChatAutomationByID(asUser(t, tc.actor), tc.automation.ID)
				if !tc.allowed {
					requireDenied(t, err)
					return
				}
				require.NoError(t, err)
				require.Equal(t, tc.automation.ID, got.ID)
			})
		}
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		everyOwner := database.GetChatAutomationsByOrganizationIDParams{OrganizationID: org.ID}
		names := func(t *testing.T, ctx context.Context, arg database.GetChatAutomationsByOrganizationIDParams) []string {
			t.Helper()
			rows, err := db.GetChatAutomationsByOrganizationID(ctx, arg)
			require.NoError(t, err)
			var out []string
			for _, row := range rows {
				// The Insert cases add rows next to these.
				if row.ID == memberAutomation.ID || row.ID == otherMemberAutomation.ID || row.ID == orphanedAutomation.ID {
					out = append(out, row.Name)
				}
			}
			return out
		}
		for _, tc := range []struct {
			name  string
			actor database.User
			arg   database.GetChatAutomationsByOrganizationIDParams
			want  []string
		}{
			{name: "MemberSeesOwn", actor: member, arg: everyOwner, want: []string{"member"}},
			{name: "OwnerWithoutAgentsAccessSeesNone", actor: memberWithoutAgentsAccess, arg: database.GetChatAutomationsByOrganizationIDParams{
				OrganizationID: noAgentsOrg.ID,
			}},
			{name: "OrgAdminSeesAll", actor: orgAdmin, arg: everyOwner, want: []string{"member", "other-member"}},
			{name: "OrgAdminOfAnotherOrganizationSeesNone", actor: otherOrgAdmin, arg: everyOwner},
			{name: "DeploymentOwnerSeesAll", actor: deploymentOwner, arg: everyOwner, want: []string{"member", "other-member"}},
			// The owner filter and the RBAC filter both apply.
			{name: "OrgAdminFiltersByOwner", actor: orgAdmin, want: []string{"other-member"}, arg: database.GetChatAutomationsByOrganizationIDParams{
				OrganizationID: org.ID, OwnerID: otherMember.ID,
			}},
			{name: "MemberFiltersByAnotherOwner", actor: member, arg: database.GetChatAutomationsByOrganizationIDParams{
				OrganizationID: org.ID, OwnerID: otherMember.ID,
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				require.ElementsMatch(t, tc.want, names(t, asUser(t, tc.actor), tc.arg))
			})
		}
	})

	t.Run("Count", func(t *testing.T) {
		t.Parallel()
		// The count spans organizations. A member's grants are scoped to
		// an organization, so the member cannot run it. The service runs
		// it as a system subject.
		_, err := db.CountChatAutomationsByOwnerID(asUser(t, member), member.ID)
		requireDenied(t, err)
		_, err = db.CountChatAutomationsByOwnerID(asUser(t, orgAdmin), member.ID)
		requireDenied(t, err)

		//nolint:gocritic // The count is a system read.
		count, err := db.CountChatAutomationsByOwnerID(dbauthz.AsChatd(testutil.Context(t, testutil.WaitLong)), otherMember.ID)
		require.NoError(t, err)
		require.EqualValues(t, 1, count)
		count, err = db.CountChatAutomationsByOwnerID(asUser(t, deploymentOwner), otherMember.ID)
		require.NoError(t, err)
		require.EqualValues(t, 1, count)
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name    string
			owner   database.User
			actor   database.User
			allowed bool
		}{
			{name: "Owner", owner: member, actor: member, allowed: true},
			{name: "AnotherMember", owner: member, actor: otherMember},
			{name: "OrgAdmin", owner: member, actor: orgAdmin, allowed: true},
			{name: "OrgAdminOfAnotherOrganization", owner: member, actor: otherOrgAdmin},
			{name: "DeploymentOwner", owner: member, actor: deploymentOwner, allowed: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				automation := dbgen.ChatAutomation(t, db, database.ChatAutomation{OrganizationID: org.ID, OwnerID: tc.owner.ID})
				err := db.DeleteChatAutomationByID(asUser(t, tc.actor), automation.ID)
				_, getErr := rawDB.GetChatAutomationByID(testutil.Context(t, testutil.WaitLong), automation.ID)
				if !tc.allowed {
					requireDenied(t, err)
					require.NoError(t, getErr, "the automation must still exist")
					return
				}
				require.NoError(t, err)
				require.ErrorIs(t, getErr, sql.ErrNoRows)
			})
		}
	})

	// A request must pass both the role grants and the scope of its API
	// key. No scope that a user can request covers chat automations.
	t.Run("ScopedAPIKey", func(t *testing.T) {
		t.Parallel()
		for _, name := range rbac.ExternalScopeNames() {
			if name == string(rbac.ScopeAll) {
				continue
			}
			ctx := as(t, member, rbac.ScopeName(name))
			_, err := db.GetChatAutomationByID(ctx, memberAutomation.ID)
			requireDenied(t, err)
			rows, err := db.GetChatAutomationsByOrganizationID(ctx, database.GetChatAutomationsByOrganizationIDParams{OrganizationID: org.ID})
			require.NoError(t, err, name)
			require.Empty(t, rows, "scope %s must not list automations", name)
			_, err = db.InsertChatAutomation(ctx, webhook(member))
			requireDenied(t, err)
			requireDenied(t, db.DeleteChatAutomationByID(ctx, memberAutomation.ID))
		}
	})

	// chatd reads automations for the scheduler and the webhook lookup. It
	// creates and deletes none.
	t.Run("Chatd", func(t *testing.T) {
		t.Parallel()
		//nolint:gocritic // This test is about the chatd subject.
		ctx := dbauthz.AsChatd(testutil.Context(t, testutil.WaitLong))
		got, err := db.GetChatAutomationByID(ctx, memberAutomation.ID)
		require.NoError(t, err)
		require.Equal(t, memberAutomation.ID, got.ID)
		rows, err := db.GetChatAutomationsByOrganizationID(ctx, database.GetChatAutomationsByOrganizationIDParams{OrganizationID: org.ID})
		require.NoError(t, err)
		require.NotEmpty(t, rows)

		_, err = db.InsertChatAutomation(ctx, webhook(member))
		requireDenied(t, err)
		requireDenied(t, db.DeleteChatAutomationByID(ctx, memberAutomation.ID))

		// The insert above stops at the owner check, so ask RBAC directly.
		subject, ok := dbauthz.ActorFromContext(ctx)
		require.True(t, ok)
		require.Error(t, authorizer.Authorize(ctx, subject, policy.ActionCreate, memberAutomation.RBACObject()),
			"chatd creates no automation")
		require.NoError(t, authorizer.Authorize(ctx, subject, policy.ActionUpdate, memberAutomation.RBACObject()),
			"chatd updates the next run time and the last run")
	})
}
