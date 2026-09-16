package rbac_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

// BenchmarkRBACManyOrgs measures authorization cost for a subject that is a
// member of many organizations. Partial evaluation (Prepare) and, depending
// on the policy shape, full evaluation (Authorize) both scale with the number
// of org-scoped roles the subject carries (see #21890).
//
// Run on two branches and compare with benchstat:
//
//	go test -run '^$' -bench '^BenchmarkRBACManyOrgs$' -benchmem -count 6 ./coderd/rbac
func BenchmarkRBACManyOrgs(b *testing.B) {
	orgCounts := []int{1, 5, 10, 50, 100}

	for _, n := range orgCounts {
		orgs := make([]uuid.UUID, n)
		for i := range orgs {
			orgs[i] = uuid.New()
		}

		userID := uuid.New()

		// Pre-expanded roles with a cached AST value mirror the subject a
		// real request carries after httpmw resolves it, so the benchmark
		// isolates policy evaluation rather than role expansion.
		member, err := rbac.RoleByName(rbac.RoleMember())
		require.NoError(b, err)
		roles := make(rbac.Roles, 0, n+1)
		roles = append(roles, member)
		// Org-scoped built-in roles are not resolvable via RoleByName, so
		// build the organization-member system role directly, the same way
		// rolestore materializes it.
		memberPerms := rbac.OrgMemberPermissions(rbac.OrgSettings{})
		for _, org := range orgs {
			roles = append(roles, rbac.Role{
				Identifier: rbac.RoleIdentifier{Name: rbac.RoleOrgMember(), OrganizationID: org},
				ByOrgID: map[string]rbac.OrgPermissions{
					org.String(): {
						Org:    memberPerms.Org,
						Member: memberPerms.Member,
					},
				},
			})
		}

		subject := rbac.Subject{
			ID:    userID.String(),
			Roles: roles,
			Scope: rbac.ScopeAll,
		}.WithCachedASTValue()

		// An owned workspace in the subject's last org exercises the
		// org_member path.
		object := rbac.ResourceWorkspace.
			WithID(uuid.New()).
			InOrg(orgs[n-1]).
			WithOwner(userID.String())

		// No caching wrapper: measure the raw evaluation cost.
		authorizer := rbac.NewAuthorizer(prometheus.NewRegistry())
		ctx := context.Background()

		b.Run(fmt.Sprintf("Authorize/orgs=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if err := authorizer.Authorize(ctx, subject, policy.ActionRead, object); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(fmt.Sprintf("Prepare/orgs=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := authorizer.Prepare(ctx, subject, policy.ActionRead, rbac.ResourceWorkspace.Type); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(fmt.Sprintf("PrepareAndCompile/orgs=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				prepared, err := authorizer.Prepare(ctx, subject, policy.ActionRead, rbac.ResourceWorkspace.Type)
				if err != nil {
					b.Fatal(err)
				}
				if _, err := prepared.CompileToSQL(ctx, rbac.ConfigWorkspaces()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
