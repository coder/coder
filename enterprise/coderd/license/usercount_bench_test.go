package license_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/enterprise/coderd/license"
)

// BenchmarkCountWorkspaceCapableUsers measures how workspace-capable seat
// counting scales along its two cost axes: the number of eligible active
// users (row fetch and per-row signature work) and the number of unique
// role sets (role expansion and rego evaluation, one per set).
//
// Scenarios (users/orgs/roles are seeded in bulk via SQL):
//
//   - Uniform: one org, half gateways, half workspace-capable. Unique
//     role sets stay constant, so this isolates per-user row cost.
//   - ManyOrgs: users spread evenly across orgs, plain members. Unique
//     role sets scale with org count.
//   - UniquePairs: every user belongs to a distinct pair of orgs, so
//     every user is a unique role set. Worst-case rego evaluation with
//     builtin roles only.
//   - CustomRoles: users hold org-scoped custom roles round-robin.
//     Exercises the custom-role prefetch and expansion path.
//
// Run with:
//
//	go test ./enterprise/coderd/license/ -bench BenchmarkCountWorkspaceCapableUsers -benchtime 5x -run '^$' -v
func BenchmarkCountWorkspaceCapableUsers(b *testing.B) {
	ctx := context.Background()
	authorizer := rbac.NewCachingAuthorizer(prometheus.NewRegistry())
	// Discard logs: the per-count Info line and its fields are not what
	// is being measured.
	logger := slog.Make()

	for _, scenario := range []benchScenario{
		{name: "Uniform/1k", users: 1_000, orgs: 1},
		{name: "Uniform/10k", users: 10_000, orgs: 1},
		{name: "Uniform/50k", users: 50_000, orgs: 1},
		{name: "ManyOrgs/10k-100orgs", users: 10_000, orgs: 100},
		{name: "UniquePairs/10k", users: 10_000, orgs: 100, uniquePairs: true},
		{name: "CustomRoles/10k-1000roles", users: 10_000, orgs: 10, customRolesPerOrg: 100},
	} {
		b.Run(scenario.name, func(b *testing.B) {
			db, _, sqlDB := dbtestutil.NewDBWithSQLDB(b)
			seedBenchUsers(ctx, b, db, sqlDB, scenario)

			b.ResetTimer()
			var count int64
			for i := 0; i < b.N; i++ {
				var err error
				count, err = license.CountWorkspaceCapableUsers(ctx, logger, db, authorizer)
				require.NoError(b, err)
			}
			b.StopTimer()
			require.NotZero(b, count, "scenario must produce capable users")
			b.ReportMetric(float64(scenario.users), "users")
			b.ReportMetric(float64(count), "capable")
		})
	}
}

type benchScenario struct {
	name  string
	users int
	orgs  int
	// uniquePairs gives every user memberships in a distinct pair of
	// orgs, making every user a unique role set.
	uniquePairs bool
	// customRolesPerOrg grants each user one org-scoped custom role,
	// assigned round-robin.
	customRolesPerOrg int
}

// seedBenchUsers bulk-inserts active users and their org memberships.
// Deterministic UUIDs (zero-prefixed, numbered) let memberships be
// generated from the same series without returning inserted rows.
func seedBenchUsers(ctx context.Context, b *testing.B, db database.Store, sqlDB *sql.DB, s benchScenario) {
	b.Helper()

	orgIDs := make([]uuid.UUID, s.orgs)
	for i := range orgIDs {
		org := dbgen.Organization(b, db, database.Organization{})
		emptyOrgDefaultRoles(ctx, b, db, org)
		orgIDs[i] = org.ID
	}

	// Deterministic user IDs let membership rows be generated from the
	// same series without returning inserted rows.
	_, err := sqlDB.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION benchUserID(i bigint) RETURNS uuid AS $$
			SELECT ('00000000-0000-0000-0000-' || lpad(i::text, 12, '0'))::uuid
		$$ LANGUAGE sql IMMUTABLE;
	`)
	require.NoError(b, err)

	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO users (id, email, username, hashed_password, created_at, updated_at, status, rbac_roles, login_type)
		SELECT
			benchUserID(i),
			'bench-' || i || '@example.com',
			'bench-' || i,
			'\x'::bytea,
			now(), now(),
			'active'::user_status,
			'{}'::text[],
			'password'::login_type
		FROM generate_series(1, $1) AS i;
	`, s.users)
	require.NoError(b, err)

	orgIDText := make([]string, len(orgIDs))
	for i, id := range orgIDs {
		orgIDText[i] = id.String()
	}

	switch {
	case s.uniquePairs:
		// Membership in orgs (i mod K) and (i/K mod K): distinct pairs,
		// hence distinct role sets, for up to K^2 users. Even users hold
		// the workspace-access grant in their first org so capability
		// varies across the population.
		require.GreaterOrEqual(b, s.orgs*s.orgs, s.users, "not enough org pairs for unique role sets")
		_, err = sqlDB.ExecContext(ctx, fmt.Sprintf(`
			INSERT INTO organization_members (user_id, organization_id, created_at, updated_at, roles)
			SELECT benchUserID(i), ($3::uuid[])[(i %% $2) + 1], now(), now(),
				CASE WHEN i %% 2 = 0 THEN ARRAY['%s']::text[] ELSE '{}'::text[] END
			FROM generate_series(1, $1) AS i
			ON CONFLICT DO NOTHING;
		`, rbac.RoleOrgWorkspaceAccess()), s.users, s.orgs, pqStringArray(orgIDText))
		require.NoError(b, err)
		_, err = sqlDB.ExecContext(ctx, `
			INSERT INTO organization_members (user_id, organization_id, created_at, updated_at, roles)
			SELECT benchUserID(i), ($3::uuid[])[((i / $2) % $2) + 1], now(), now(), '{}'::text[]
			FROM generate_series(1, $1) AS i
			ON CONFLICT DO NOTHING;
		`, s.users, s.orgs, pqStringArray(orgIDText))
		require.NoError(b, err)
	case s.customRolesPerOrg > 0:
		// One workspace-create custom role per (org, slot), granted
		// round-robin: users cycle through orgs, and within an org
		// through its roles.
		_, err = sqlDB.ExecContext(ctx, `
			INSERT INTO custom_roles (name, display_name, organization_id, org_permissions)
			SELECT
				'bench-role-' || slot,
				'Bench Role ' || slot,
				($2::uuid[])[(slot % $3) + 1],
				'[{"negate":false,"resource_type":"workspace","action":"create"}]'::jsonb
			FROM generate_series(0, $1 - 1) AS slot;
		`, s.orgs*s.customRolesPerOrg, pqStringArray(orgIDText), s.orgs)
		require.NoError(b, err)
		_, err = sqlDB.ExecContext(ctx, `
			INSERT INTO organization_members (user_id, organization_id, created_at, updated_at, roles)
			SELECT
				benchUserID(i),
				($2::uuid[])[(i % $3) + 1],
				now(), now(),
				ARRAY['bench-role-' || ((i % ($3 * $4) / $3) * $3 + (i % $3))]::text[]
			FROM generate_series(1, $1) AS i;
		`, s.users, pqStringArray(orgIDText), s.orgs, s.customRolesPerOrg)
		require.NoError(b, err)
	default:
		// Round-robin plain membership; every even user additionally
		// holds the workspace-access grant so capability varies.
		_, err = sqlDB.ExecContext(ctx, fmt.Sprintf(`
			INSERT INTO organization_members (user_id, organization_id, created_at, updated_at, roles)
			SELECT
				benchUserID(i),
				($2::uuid[])[(i %% $3) + 1],
				now(), now(),
				CASE WHEN i %% 2 = 0 THEN ARRAY['%s']::text[] ELSE '{}'::text[] END
			FROM generate_series(1, $1) AS i;
		`, rbac.RoleOrgWorkspaceAccess()), s.users, pqStringArray(orgIDText), s.orgs)
		require.NoError(b, err)
	}

	// Bulk inserts leave planner statistics claiming near-empty tables,
	// which makes the roles query fall into a nested-loop plan that
	// re-executes its aggregation per user row. Refresh them the way
	// autovacuum would have in a live deployment.
	_, err = sqlDB.ExecContext(ctx, `ANALYZE users; ANALYZE organization_members; ANALYZE organizations; ANALYZE custom_roles;`)
	require.NoError(b, err)
}

func emptyOrgDefaultRoles(ctx context.Context, b *testing.B, db database.Store, org database.Organization) {
	b.Helper()
	_, err := db.UpdateOrganization(ctx, database.UpdateOrganizationParams{
		ID:                    org.ID,
		UpdatedAt:             org.UpdatedAt,
		Name:                  org.Name,
		DisplayName:           org.DisplayName,
		Description:           org.Description,
		Icon:                  org.Icon,
		DefaultOrgMemberRoles: []string{},
	})
	require.NoError(b, err)
}

func pqStringArray(elems []string) string {
	out := "{"
	for i, e := range elems {
		if i > 0 {
			out += ","
		}
		out += e
	}
	return out + "}"
}
