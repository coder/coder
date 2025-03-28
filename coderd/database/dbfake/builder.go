package dbfake

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"
)

type OrganizationBuilder struct {
	t                 *testing.T
	db                database.Store
	seed              database.Organization
	allUsersAllowance int32
	members           []uuid.UUID
	groups            map[database.Group][]uuid.UUID
}

func Organization(t *testing.T, db database.Store) OrganizationBuilder {
	return OrganizationBuilder{
		t:       t,
		db:      db,
		members: []uuid.UUID{},
		groups:  make(map[database.Group][]uuid.UUID),
	}
}

type OrganizationResponse struct {
	Org           database.Organization
	AllUsersGroup database.Group
	Members       []database.OrganizationMember
	Groups        []database.Group
}

func (b OrganizationBuilder) EveryoneAllowance(allowance int) OrganizationBuilder {
	//nolint: revive // returns modified struct
	// #nosec G115 - Safe conversion as allowance is expected to be within int32 range
	b.allUsersAllowance = int32(allowance)
	return b
}

func (b OrganizationBuilder) Seed(seed database.Organization) OrganizationBuilder {
	//nolint: revive // returns modified struct
	b.seed = seed
	return b
}

func (b OrganizationBuilder) Members(users ...database.User) OrganizationBuilder {
	for _, u := range users {
		//nolint: revive // returns modified struct
		b.members = append(b.members, u.ID)
	}
	return b
}

func (b OrganizationBuilder) Group(seed database.Group, members ...database.User) OrganizationBuilder {
	//nolint: revive // returns modified struct
	b.groups[seed] = []uuid.UUID{}
	for _, u := range members {
		//nolint: revive // returns modified struct
		b.groups[seed] = append(b.groups[seed], u.ID)
	}
	return b
}

func (b OrganizationBuilder) Do() OrganizationResponse {
	org := dbgen.Organization(b.t, b.db, b.seed)

	ctx := testutil.Context(b.t, testutil.WaitShort)
	//nolint:gocritic // builder code needs perms
	ctx = dbauthz.AsSystemRestricted(ctx)
	everyone, err := b.db.InsertAllUsersGroup(ctx, org.ID)
	require.NoError(b.t, err)

	if b.allUsersAllowance > 0 {
		everyone, err = b.db.UpdateGroupByID(ctx, database.UpdateGroupByIDParams{
			Name:           everyone.Name,
			DisplayName:    everyone.DisplayName,
			AvatarURL:      everyone.AvatarURL,
			QuotaAllowance: b.allUsersAllowance,
			ID:             everyone.ID,
		})
		require.NoError(b.t, err)
	}

	members := make([]database.OrganizationMember, 0)
	if len(b.members) > 0 {
		for _, u := range b.members {
			newMem := dbgen.OrganizationMember(b.t, b.db, database.OrganizationMember{
				UserID:         u,
				OrganizationID: org.ID,
				CreatedAt:      dbtime.Now(),
				UpdatedAt:      dbtime.Now(),
				Roles:          nil,
			})
			members = append(members, newMem)
		}
	}

	groups := make([]database.Group, 0)
	if len(b.groups) > 0 {
		for g, users := range b.groups {
			g.OrganizationID = org.ID
			group := dbgen.Group(b.t, b.db, g)
			groups = append(groups, group)

			for _, u := range users {
				dbgen.GroupMember(b.t, b.db, database.GroupMemberTable{
					UserID:  u,
					GroupID: group.ID,
				})
			}
		}
	}

	return OrganizationResponse{
		Org:           org,
		AllUsersGroup: everyone,
		Members:       members,
		Groups:        groups,
	}
}
