package database_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/skills"
	"github.com/coder/coder/v2/testutil"
)

func TestUserSkillSchemaConstants(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	ctx := testutil.Context(t, testutil.WaitMedium)
	_, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	var triggerDef string
	err := sqlDB.QueryRowContext(ctx,
		`SELECT pg_get_functiondef('enforce_skills_per_owner_limit'::regproc)`,
	).Scan(&triggerDef)
	require.NoError(t, err)
	require.Contains(t, triggerDef, fmt.Sprintf(
		"skill_limit constant int := %d",
		skills.MaxPersonalSkillsPerUser,
	))

	constraints := map[database.CheckConstraint]string{
		database.CheckSkillsNameSize: fmt.Sprintf(
			"octet_length(name) <= %d",
			skills.MaxPersonalSkillNameBytes,
		),
		database.CheckSkillsNameFormat: "name ~ '^[a-z0-9]+(-[a-z0-9]+)*$'::text",
		database.CheckSkillsDescriptionSize: fmt.Sprintf(
			"octet_length(description) <= %d",
			skills.MaxPersonalSkillDescriptionBytes,
		),
		database.CheckSkillsContentSize: fmt.Sprintf(
			"octet_length(content) <= %d",
			skills.MaxPersonalSkillSizeBytes,
		),
	}
	for constraint, expected := range constraints {
		t.Run(string(constraint), func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			var constraintDef string
			err := sqlDB.QueryRowContext(ctx,
				`SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = $1`,
				constraint,
			).Scan(&constraintDef)
			require.NoError(t, err)
			require.Contains(t, constraintDef, expected)
		})
	}
}

func TestSkillOwners(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

	type owners struct {
		user, org, project uuid.NullUUID
	}
	type skillRow struct {
		owners
		name              string
		enabled           bool
		groupACL, userACL string
	}
	insertSkill := func(ctx context.Context, q interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	}, row skillRow,
	) error {
		if row.name == "" {
			row.name = "skill-" + uuid.NewString()
		}
		if row.groupACL == "" {
			row.groupACL = "{}"
		}
		if row.userACL == "" {
			row.userACL = "{}"
		}
		_, err := q.ExecContext(ctx, `
			INSERT INTO skills (id, user_id, organization_id, project_id, name, description, content, enabled, group_acl, user_acl)
			VALUES ($1, $2, $3, $4, $5, 'desc', 'body', $6, $7::jsonb, $8::jsonb)`,
			uuid.New(), row.user, row.org, row.project, row.name, row.enabled, row.groupACL, row.userACL)
		return err
	}
	countSkills := func(ctx context.Context, column string, id uuid.UUID) int {
		var count int
		//nolint:gosec // column is a fixed owner column name from this test.
		err := sqlDB.QueryRowContext(ctx, "SELECT count(*) FROM skills WHERE "+column+" = $1", id).Scan(&count)
		require.NoError(t, err)
		return count
	}
	valid := func(id uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: id, Valid: true} }
	newUser := func() uuid.UUID { return dbgen.User(t, db, database.User{}).ID }
	newOrg := func() uuid.UUID { return dbgen.Organization(t, db, database.Organization{}).ID }
	newProject := func() uuid.UUID {
		org := newOrg()
		return dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: org, OwnerID: newUser()}).ID
	}

	t.Run("ExactlyOneOwner", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		user, org, project := valid(newUser()), valid(newOrg()), valid(newProject())
		for _, o := range []owners{
			{},
			{user: user, org: org},
			{org: org, project: project},
			{user: user, project: project},
		} {
			err := insertSkill(ctx, sqlDB, skillRow{owners: o, enabled: true})
			require.True(t, database.IsCheckViolation(err, database.CheckSkillsSingleOwner), "owners %+v: %v", o, err)
		}
	})

	t.Run("ACLOnlyOnOrganizationSkills", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		grant := `{"` + uuid.NewString() + `": {"permissions": ["read"]}}`
		for _, o := range []owners{{user: valid(newUser())}, {project: valid(newProject())}} {
			err := insertSkill(ctx, sqlDB, skillRow{owners: o, groupACL: grant})
			require.True(t, database.IsCheckViolation(err, database.CheckSkillsAclOnlyOnOrganizationSkills), "group ACL %+v: %v", o, err)
			err = insertSkill(ctx, sqlDB, skillRow{owners: o, userACL: grant})
			require.True(t, database.IsCheckViolation(err, database.CheckSkillsAclOnlyOnOrganizationSkills), "user ACL %+v: %v", o, err)
		}

		org := owners{org: valid(newOrg())}
		require.NoError(t, insertSkill(ctx, sqlDB, skillRow{owners: org, groupACL: grant, userACL: grant}))
		err := insertSkill(ctx, sqlDB, skillRow{owners: org, groupACL: "[]"})
		require.True(t, database.IsCheckViolation(err, database.CheckSkillsGroupAclIsObject), err)
		err = insertSkill(ctx, sqlDB, skillRow{owners: org, userACL: "[]"})
		require.True(t, database.IsCheckViolation(err, database.CheckSkillsUserAclIsObject), err)
	})

	t.Run("CapPerOwner", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name       string
			newOwner   func() owners
			constraint database.CheckConstraint
		}{
			{"User", func() owners { return owners{user: valid(newUser())} }, "skills_per_user_limit"},
			{"Organization", func() owners { return owners{org: valid(newOrg())} }, "skills_per_organization_limit"},
			{"Project", func() owners { return owners{project: valid(newProject())} }, "skills_per_project_limit"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)
				full := tc.newOwner()
				for i := range skills.MaxPersonalSkillsPerUser {
					// Disabled rows count toward the cap.
					require.NoError(t, insertSkill(ctx, sqlDB, skillRow{owners: full, enabled: i%2 == 0}))
				}
				err := insertSkill(ctx, sqlDB, skillRow{owners: full, enabled: true})
				require.True(t, database.IsCheckViolation(err, tc.constraint), err)

				require.NoError(t, insertSkill(ctx, sqlDB, skillRow{owners: tc.newOwner(), enabled: true}))
			})
		}
	})

	t.Run("ConcurrentOrganizationInsertsHonorCap", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		org := valid(newOrg())
		for range skills.MaxPersonalSkillsPerUser - 1 {
			require.NoError(t, insertSkill(ctx, sqlDB, skillRow{owners: owners{org: org}, enabled: true}))
		}

		tx, err := sqlDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		require.NoError(t, insertSkill(ctx, tx, skillRow{owners: owners{org: org}, enabled: true}))

		// Foreign key checks from unrelated inserts take KEY SHARE on the
		// organization row, which the cap lock must not block.
		_, err = sqlDB.ExecContext(ctx, "SELECT 1 FROM organizations WHERE id = $1 FOR KEY SHARE NOWAIT", org.UUID)
		require.NoError(t, err)

		concurrent := make(chan error, 1)
		go func() {
			concurrent <- insertSkill(ctx, sqlDB, skillRow{owners: owners{org: org}, enabled: true})
		}()
		// Commit only after the concurrent insert waits on the cap lock, so
		// it cannot finish before the transaction by chance.
		require.Eventually(t, func() bool {
			if len(concurrent) > 0 {
				return true
			}
			var waiting bool
			err := sqlDB.QueryRowContext(ctx, `SELECT EXISTS (
				SELECT 1 FROM pg_stat_activity
				WHERE datname = current_database() AND wait_event_type = 'Lock' AND query LIKE '%INSERT INTO skills%'
			)`).Scan(&waiting)
			return err == nil && waiting
		}, testutil.WaitShort, testutil.IntervalFast)
		require.NoError(t, tx.Commit())

		err = testutil.RequireReceive(ctx, t, concurrent)
		require.True(t, database.IsCheckViolation(err, "skills_per_organization_limit"), err)
		require.Equal(t, skills.MaxPersonalSkillsPerUser, countSkills(ctx, "organization_id", org.UUID))
	})

	t.Run("NamesUniquePerOwner", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		const name = "shared-name"
		user, org, otherOrg, project := valid(newUser()), valid(newOrg()), valid(newOrg()), valid(newProject())
		for _, o := range []owners{{user: user}, {org: org}, {org: otherOrg}, {project: project}} {
			require.NoError(t, insertSkill(ctx, sqlDB, skillRow{owners: o, name: name}))
		}

		err := insertSkill(ctx, sqlDB, skillRow{owners: owners{user: user}, name: name})
		require.True(t, database.IsUniqueViolation(err, database.UniqueSkillsUserIDNameIndex), err)
		err = insertSkill(ctx, sqlDB, skillRow{owners: owners{org: org}, name: name})
		require.True(t, database.IsUniqueViolation(err, database.UniqueSkillsOrganizationIDNameIndex), err)
		err = insertSkill(ctx, sqlDB, skillRow{owners: owners{project: project}, name: name})
		require.True(t, database.IsUniqueViolation(err, database.UniqueSkillsProjectIDNameIndex), err)
	})

	t.Run("MemberRemovalDropsDirectGrants", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		org, otherOrg := newOrg(), newOrg()
		removed, kept := newUser(), newUser()
		for _, o := range []uuid.UUID{org, otherOrg} {
			dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: o, UserID: removed})
			grants := fmt.Sprintf(`{%q: {"permissions": ["read"]}, %q: {"permissions": ["read"]}}`, removed, kept)
			require.NoError(t, insertSkill(ctx, sqlDB, skillRow{owners: owners{org: valid(o)}, userACL: grants}))
		}

		require.NoError(t, db.DeleteOrganizationMember(ctx, database.DeleteOrganizationMemberParams{OrganizationID: org, UserID: removed}))

		hasGrant := func(org, user uuid.UUID) bool {
			t.Helper()
			var has bool
			err := sqlDB.QueryRowContext(ctx, "SELECT user_acl ? $2 FROM skills WHERE organization_id = $1", org, user.String()).Scan(&has)
			require.NoError(t, err)
			return has
		}
		require.False(t, hasGrant(org, removed))
		require.True(t, hasGrant(org, kept))
		require.True(t, hasGrant(otherOrg, removed))
	})

	t.Run("OwnerDeletionRemovesOnlyOwnRows", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		user, deletedUser := newUser(), newUser()
		org, deletedOrg := newOrg(), newOrg()
		project := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: org, OwnerID: deletedUser}).ID
		deletedProject := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: org, OwnerID: deletedUser}).ID
		for _, o := range []owners{
			{user: valid(user)},
			{user: valid(deletedUser)},
			{org: valid(org)},
			{org: valid(deletedOrg)},
			{project: valid(project)},
			{project: valid(deletedProject)},
		} {
			require.NoError(t, insertSkill(ctx, sqlDB, skillRow{owners: o}))
		}

		_, err := sqlDB.ExecContext(ctx, "UPDATE users SET deleted = true WHERE id = $1", deletedUser)
		require.NoError(t, err)
		_, err = sqlDB.ExecContext(ctx, "DELETE FROM organizations WHERE id = $1", deletedOrg)
		require.NoError(t, err)
		require.NoError(t, db.DeleteChatProjectByID(ctx, deletedProject))

		require.Equal(t, 1, countSkills(ctx, "user_id", user))
		require.Equal(t, 0, countSkills(ctx, "user_id", deletedUser))
		require.Equal(t, 1, countSkills(ctx, "organization_id", org))
		require.Equal(t, 0, countSkills(ctx, "organization_id", deletedOrg))
		// The soft-deleted user's project keeps its skills.
		require.Equal(t, 1, countSkills(ctx, "project_id", project))
		require.Equal(t, 0, countSkills(ctx, "project_id", deletedProject))
	})
}

// TestOrganizationSkillQueries runs the organization skill queries that only
// execute against Postgres here: the SQL-filtered list, whose regosql
// converter must agree with the Rego ACL rules, and the update and delete
// statements.
func TestOrganizationSkillQueries(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)
	authzDB := dbauthz.New(db, rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()), slogtest.Make(t, nil), coderdtest.AccessControlStorePointer())

	org := dbgen.Organization(t, db, database.Organization{})
	otherOrg := dbgen.Organization(t, db, database.Organization{})
	group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})
	newUser := func(siteRoles []string, orgID uuid.UUID, orgRoles ...string) uuid.UUID {
		user := dbgen.User(t, db, database.User{RBACRoles: siteRoles})
		if orgID != uuid.Nil {
			dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: orgID, Roles: orgRoles})
		}
		return user.ID
	}
	var (
		siteOwner      = newUser([]string{rbac.RoleOwner().String()}, uuid.Nil)
		orgAdmin       = newUser(nil, org.ID, rbac.RoleOrgAdmin())
		groupMember    = newUser(nil, org.ID)
		grantedMember  = newUser(nil, org.ID)
		grantedOutside = newUser(nil, uuid.Nil)
	)
	dbgen.GroupMember(t, db, database.GroupMemberTable{UserID: groupMember, GroupID: group.ID})

	read := database.ChatACLEntry{Permissions: []policy.Action{policy.ActionRead}}
	orgSkill := func(groupACL, userACL database.ChatACL) database.Skill {
		return dbgen.OrganizationSkill(t, db, database.Skill{
			OrganizationID: uuid.NullUUID{UUID: org.ID, Valid: true},
			GroupACL:       groupACL,
			UserACL:        userACL,
		})
	}
	everyoneSkill := orgSkill(database.ChatACL{org.ID.String(): read}, nil)
	groupSkill := orgSkill(database.ChatACL{group.ID.String(): read}, nil)
	userSkill := orgSkill(nil, database.ChatACL{grantedMember.String(): read, grantedOutside.String(): read})
	privateSkill := orgSkill(nil, nil)
	dbgen.OrganizationSkill(t, db, database.Skill{
		OrganizationID: uuid.NullUUID{UUID: otherOrg.ID, Valid: true},
		GroupACL:       database.ChatACL{otherOrg.ID.String(): read},
	})

	as := func(userID uuid.UUID) context.Context {
		subject, _, err := httpmw.UserRBACSubject(ctx, db, userID, rbac.ExpandableScope(rbac.ScopeAll))
		require.NoError(t, err)
		return dbauthz.As(ctx, subject)
	}
	all := []uuid.UUID{everyoneSkill.ID, groupSkill.ID, userSkill.ID, privateSkill.ID}
	for name, tc := range map[string]struct {
		user uuid.UUID
		want []uuid.UUID
	}{
		"SiteOwner":      {siteOwner, all},
		"OrgAdmin":       {orgAdmin, all},
		"GroupMember":    {groupMember, []uuid.UUID{everyoneSkill.ID, groupSkill.ID}},
		"GrantedMember":  {grantedMember, []uuid.UUID{everyoneSkill.ID, userSkill.ID}},
		"GrantedOutside": {grantedOutside, nil},
	} {
		rows, err := authzDB.ListOrganizationSkillMetadataByOrganizationID(as(tc.user), org.ID)
		require.NoError(t, err, name)
		got := make([]uuid.UUID, 0, len(rows))
		for _, row := range rows {
			got = append(got, row.ID)
		}
		require.ElementsMatch(t, tc.want, got, name)
	}

	updated, err := authzDB.UpdateOrganizationSkillByOrganizationIDAndName(as(orgAdmin), database.UpdateOrganizationSkillByOrganizationIDAndNameParams{
		OrganizationID: org.ID, Name: groupSkill.Name, Enabled: sql.NullBool{Bool: false, Valid: true},
	})
	require.NoError(t, err)
	require.False(t, updated.Enabled)
	require.Equal(t, groupSkill.Content, updated.Content)
	deleted, err := authzDB.DeleteOrganizationSkillByOrganizationIDAndName(as(orgAdmin), database.DeleteOrganizationSkillByOrganizationIDAndNameParams{
		OrganizationID: org.ID, Name: groupSkill.Name,
	})
	require.NoError(t, err)
	require.Equal(t, groupSkill.ID, deleted.ID)
}
