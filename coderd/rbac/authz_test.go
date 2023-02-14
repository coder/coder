package rbac_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/rbac"
)

type benchmarkCase struct {
	Name  string
	Actor rbac.Subject
}

// benchmarkUserCases builds a set of users with different roles and groups.
// The user id used as the subject and the orgs used to build the roles are
// returned.
func benchmarkUserCases() (cases []benchmarkCase, users uuid.UUID, orgs []uuid.UUID) {
	orgs = []uuid.UUID{
		uuid.MustParse("bf7b72bd-a2b1-4ef2-962c-1d698e0483f6"),
		uuid.MustParse("e4660c6f-b9de-422d-9578-cd888983a795"),
		uuid.MustParse("fb13d477-06f4-42d9-b957-f6b89bd63515"),
	}

	user := uuid.MustParse("10d03e62-7703-4df5-a358-4f76577d4e2f")
	// noiseGroups are just added to add noise to the authorize call. They
	// never match an object's list of groups.
	noiseGroups := []string{uuid.NewString(), uuid.NewString(), uuid.NewString()}

	benchCases := []benchmarkCase{
		{
			Name: "NoRoles",
			Actor: rbac.Subject{
				ID:    user.String(),
				Roles: rbac.RoleNames{},
				Scope: rbac.ScopeAll,
			},
		},
		{
			Name: "Admin",
			Actor: rbac.Subject{
				// Give some extra roles that an admin might have
				Roles:  rbac.RoleNames{rbac.RoleOrgMember(orgs[0]), "auditor", rbac.RoleOwner(), rbac.RoleMember()},
				ID:     user.String(),
				Scope:  rbac.ScopeAll,
				Groups: noiseGroups,
			},
		},
		{
			Name: "OrgAdmin",
			Actor: rbac.Subject{
				Roles:  rbac.RoleNames{rbac.RoleOrgMember(orgs[0]), rbac.RoleOrgAdmin(orgs[0]), rbac.RoleMember()},
				ID:     user.String(),
				Scope:  rbac.ScopeAll,
				Groups: noiseGroups,
			},
		},
		{
			Name: "OrgMember",
			Actor: rbac.Subject{
				// Member of 2 orgs
				Roles:  rbac.RoleNames{rbac.RoleOrgMember(orgs[0]), rbac.RoleOrgMember(orgs[1]), rbac.RoleMember()},
				ID:     user.String(),
				Scope:  rbac.ScopeAll,
				Groups: noiseGroups,
			},
		},
		{
			Name: "ManyRoles",
			Actor: rbac.Subject{
				// Admin of many orgs
				Roles: rbac.RoleNames{
					rbac.RoleOrgMember(orgs[0]), rbac.RoleOrgAdmin(orgs[0]),
					rbac.RoleOrgMember(orgs[1]), rbac.RoleOrgAdmin(orgs[1]),
					rbac.RoleOrgMember(orgs[2]), rbac.RoleOrgAdmin(orgs[2]),
					rbac.RoleMember(),
				},
				ID:     user.String(),
				Scope:  rbac.ScopeAll,
				Groups: noiseGroups,
			},
		},
		{
			Name: "AdminWithScope",
			Actor: rbac.Subject{
				// Give some extra roles that an admin might have
				Roles:  rbac.RoleNames{rbac.RoleOrgMember(orgs[0]), "auditor", rbac.RoleOwner(), rbac.RoleMember()},
				ID:     user.String(),
				Scope:  rbac.ScopeApplicationConnect,
				Groups: noiseGroups,
			},
		},
	}
	return benchCases, users, orgs
}

// BenchmarkRBACAuthorize benchmarks the rbac.Authorize method.
//
//	go test -bench BenchmarkRBACAuthorize -benchmem -memprofile memprofile.out -cpuprofile profile.out
func BenchmarkRBACAuthorize(b *testing.B) {
	benchCases, user, orgs := benchmarkUserCases()
	users := append([]uuid.UUID{},
		user,
		uuid.MustParse("4ca78b1d-f2d2-4168-9d76-cd93b51c6c1e"),
		uuid.MustParse("0632b012-49e0-4d70-a5b3-f4398f1dcd52"),
		uuid.MustParse("70dbaa7a-ea9c-4f68-a781-97b08af8461d"),
	)
	authorizer := rbac.NewCachingAuthorizer(prometheus.NewRegistry())
	// This benchmarks all the simple cases using just user permissions. Groups
	// are added as noise, but do not do anything.
	for _, c := range benchCases {
		b.Run(c.Name, func(b *testing.B) {
			objects := benchmarkSetup(orgs, users, b.N)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				allowed := authorizer.Authorize(context.Background(), c.Actor, rbac.ActionRead, objects[b.N%len(objects)])
				var _ = allowed
			}
		})
	}
}

// BenchmarkRBACAuthorizeGroups benchmarks the rbac.Authorize method and leverages
// groups for authorizing rather than the permissions/roles.
//
//	go test -bench BenchmarkRBACAuthorizeGroups -benchmem -memprofile memprofile.out -cpuprofile profile.out
func BenchmarkRBACAuthorizeGroups(b *testing.B) {
	benchCases, user, orgs := benchmarkUserCases()
	users := append([]uuid.UUID{},
		user,
		uuid.MustParse("4ca78b1d-f2d2-4168-9d76-cd93b51c6c1e"),
		uuid.MustParse("0632b012-49e0-4d70-a5b3-f4398f1dcd52"),
		uuid.MustParse("70dbaa7a-ea9c-4f68-a781-97b08af8461d"),
	)
	authorizer := rbac.NewCachingAuthorizer(prometheus.NewRegistry())

	// Same benchmark cases, but this time groups will be used to match.
	// Some '*' permissions will still match, but using a fake action reduces
	// the chance.
	neverMatchAction := rbac.Action("never-match-action")
	for _, c := range benchCases {
		b.Run(c.Name+"GroupACL", func(b *testing.B) {
			userGroupAllow := uuid.NewString()
			c.Actor.Groups = append(c.Actor.Groups, userGroupAllow)
			c.Actor.Scope = rbac.ScopeAll
			objects := benchmarkSetup(orgs, users, b.N, func(object rbac.Object) rbac.Object {
				m := map[string][]rbac.Action{
					// Add the user's group
					// Noise
					uuid.NewString(): {rbac.ActionCreate, rbac.ActionRead, rbac.ActionUpdate, rbac.ActionDelete},
					uuid.NewString(): {rbac.ActionCreate, rbac.ActionRead, rbac.ActionUpdate},
					uuid.NewString(): {rbac.ActionCreate, rbac.ActionRead},
					uuid.NewString(): {rbac.ActionCreate},
					uuid.NewString(): {rbac.ActionRead, rbac.ActionUpdate, rbac.ActionDelete},
					uuid.NewString(): {rbac.ActionRead, rbac.ActionUpdate},
				}
				for _, g := range c.Actor.Groups {
					// Every group the user is in will be added, but it will not match the perms. This makes the
					// authorizer look at many groups before finding the one that matches.
					m[g] = []rbac.Action{rbac.ActionCreate, rbac.ActionRead, rbac.ActionUpdate, rbac.ActionDelete}
				}
				// This is the only group that will give permission.
				m[userGroupAllow] = []rbac.Action{neverMatchAction}
				return object.WithGroupACL(m)
			})
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				allowed := authorizer.Authorize(context.Background(), c.Actor, neverMatchAction, objects[b.N%len(objects)])
				var _ = allowed
			}
		})
	}
}

// BenchmarkRBACFilter benchmarks the rbac.Filter method.
//
//	go test -bench BenchmarkRBACFilter -benchmem -memprofile memprofile.out -cpuprofile profile.out
func BenchmarkRBACFilter(b *testing.B) {
	benchCases, user, orgs := benchmarkUserCases()
	users := append([]uuid.UUID{},
		user,
		uuid.MustParse("4ca78b1d-f2d2-4168-9d76-cd93b51c6c1e"),
		uuid.MustParse("0632b012-49e0-4d70-a5b3-f4398f1dcd52"),
		uuid.MustParse("70dbaa7a-ea9c-4f68-a781-97b08af8461d"),
	)

	authorizer := rbac.NewCachingAuthorizer(prometheus.NewRegistry())

	for _, c := range benchCases {
		b.Run("PrepareOnly-"+c.Name, func(b *testing.B) {
			obType := rbac.ResourceWorkspace.Type
			for i := 0; i < b.N; i++ {
				_, err := authorizer.Prepare(context.Background(), c.Actor, rbac.ActionRead, obType)
				require.NoError(b, err)
			}
		})
	}

	for _, c := range benchCases {
		b.Run(c.Name, func(b *testing.B) {
			objects := benchmarkSetup(orgs, users, b.N)
			b.ResetTimer()
			allowed, err := rbac.Filter(context.Background(), authorizer, c.Actor, rbac.ActionRead, objects)
			require.NoError(b, err)
			var _ = allowed
		})
	}
}

func benchmarkSetup(orgs []uuid.UUID, users []uuid.UUID, size int, opts ...func(object rbac.Object) rbac.Object) []rbac.Object {
	// Create a "random" but deterministic set of objects.
	aclList := map[string][]rbac.Action{
		uuid.NewString(): {rbac.ActionRead, rbac.ActionUpdate},
		uuid.NewString(): {rbac.ActionCreate},
	}
	objectList := make([]rbac.Object, size)
	for i := range objectList {
		objectList[i] = rbac.ResourceWorkspace.
			WithID(uuid.New()).
			InOrg(orgs[i%len(orgs)]).
			WithOwner(users[i%len(users)].String()).
			WithACLUserList(aclList).
			WithGroupACL(aclList)

		for _, opt := range opts {
			objectList[i] = opt(objectList[i])
		}
	}

	return objectList
}
