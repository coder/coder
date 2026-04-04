package coderdtest

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/userpassword"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// UsersPagination creates a set of users for testing pagination.  It can be
// used to test paginating both users and group members.
func UsersPagination(
	ctx context.Context,
	t *testing.T,
	client *codersdk.Client,
	setup func(users []codersdk.User),
	fetch func(req codersdk.UsersRequest) ([]codersdk.ReducedUser, int),
) {
	t.Helper()

	firstUser, err := client.User(ctx, codersdk.Me)
	require.NoError(t, err, "fetch me")

	count := 10
	users := make([]codersdk.User, count)
	orgID := firstUser.OrganizationIDs[0]
	users[0] = firstUser
	for i := range count - 1 {
		_, user := CreateAnotherUserMutators(t, client, orgID, nil, func(r *codersdk.CreateUserRequestWithOrgs) {
			if i < 5 {
				r.Name = fmt.Sprintf("before%d", i)
			} else {
				r.Name = fmt.Sprintf("after%d", i)
			}
		})
		users[i+1] = user
	}

	slices.SortFunc(users, func(a, b codersdk.User) int {
		return slice.Ascending(strings.ToLower(a.Username), strings.ToLower(b.Username))
	})

	if setup != nil {
		setup(users)
	}

	gotUsers, gotCount := fetch(codersdk.UsersRequest{})
	require.Len(t, gotUsers, count)
	require.Equal(t, gotCount, count)

	gotUsers, gotCount = fetch(codersdk.UsersRequest{
		Pagination: codersdk.Pagination{
			Limit: 1,
		},
	})
	require.Len(t, gotUsers, 1)
	require.Equal(t, gotCount, count)

	gotUsers, gotCount = fetch(codersdk.UsersRequest{
		Pagination: codersdk.Pagination{
			Offset: 1,
		},
	})
	require.Len(t, gotUsers, count-1)
	require.Equal(t, gotCount, count)

	gotUsers, gotCount = fetch(codersdk.UsersRequest{
		Pagination: codersdk.Pagination{
			Limit:  1,
			Offset: 1,
		},
	})
	require.Len(t, gotUsers, 1)
	require.Equal(t, gotCount, count)

	// If offset is higher than the count postgres returns an empty array
	// and not an ErrNoRows error.
	gotUsers, gotCount = fetch(codersdk.UsersRequest{
		Pagination: codersdk.Pagination{
			Offset: count + 1,
		},
	})
	require.Len(t, gotUsers, 0)
	require.Equal(t, gotCount, 0)

	// Check that AfterID works.
	gotUsers, gotCount = fetch(codersdk.UsersRequest{
		Pagination: codersdk.Pagination{
			AfterID: users[5].ID,
		},
	})
	require.NoError(t, err)
	require.Len(t, gotUsers, 4)
	require.Equal(t, gotCount, 4)

	// Check we can paginate a filtered response.
	gotUsers, gotCount = fetch(codersdk.UsersRequest{
		SearchQuery: "name:after",
		Pagination: codersdk.Pagination{
			Limit:  1,
			Offset: 1,
		},
	})
	require.NoError(t, err)
	require.Len(t, gotUsers, 1)
	require.Equal(t, gotCount, 4)
	require.Contains(t, gotUsers[0].Name, "after")
}

type UsersFilterOptions struct {
	CreateServiceAccounts bool
}

// UsersFilter creates a set of users to run various filters against for
// testing.  It can be used to test filtering both users and group members.
func UsersFilter(
	setupCtx context.Context,
	t *testing.T,
	client *codersdk.Client,
	db database.Store,
	options *UsersFilterOptions,
	setup func(users []codersdk.User),
	fetch func(ctx context.Context, req codersdk.UsersRequest) []codersdk.ReducedUser,
) {
	t.Helper()

	if options == nil {
		options = &UsersFilterOptions{}
	}

	firstUser, err := client.User(setupCtx, codersdk.Me)
	require.NoError(t, err, "fetch me")

	// Noon on Jan 18 is the "now" for this test for last_seen timestamps.
	// All these values are equal
	// 2023-01-18T12:00:00Z (UTC)
	// 2023-01-18T07:00:00-05:00 (America/New_York)
	// 2023-01-18T13:00:00+01:00 (Europe/Madrid)
	// 2023-01-16T00:00:00+12:00 (Asia/Anadyr)
	lastSeenNow := time.Date(2023, 1, 18, 12, 0, 0, 0, time.UTC)
	users := make([]codersdk.User, 0)
	users = append(users, firstUser)
	orgID := firstUser.OrganizationIDs[0]
	for i := range 15 {
		roles := []rbac.RoleIdentifier{}
		if i%2 == 0 {
			roles = append(roles, rbac.RoleTemplateAdmin(), rbac.RoleUserAdmin())
		}
		if i%3 == 0 {
			roles = append(roles, rbac.RoleAuditor())
		}
		userClient, userData := CreateAnotherUserMutators(t, client, orgID, roles, func(r *codersdk.CreateUserRequestWithOrgs) {
			switch {
			case i%7 == 0:
				r.Username += fmt.Sprintf("-gh%d", i)
				r.UserLoginType = codersdk.LoginTypeGithub
				r.Password = ""
			case i%6 == 0:
				r.UserLoginType = codersdk.LoginTypeOIDC
				r.Password = ""
			default:
				r.UserLoginType = codersdk.LoginTypePassword
			}
		})

		// Set the last seen for each user to a unique day
		// nolint:gocritic // Setting up unit test data.
		_, err := db.UpdateUserLastSeenAt(dbauthz.AsSystemRestricted(setupCtx), database.UpdateUserLastSeenAtParams{
			ID:         userData.ID,
			LastSeenAt: lastSeenNow.Add(-1 * time.Hour * 24 * time.Duration(i)),
			UpdatedAt:  time.Now(),
		})
		require.NoError(t, err, "set a last seen")

		// Set a github user ID for github login types.
		if i%7 == 0 {
			// nolint:gocritic // Setting up unit test data.
			err = db.UpdateUserGithubComUserID(dbauthz.AsSystemRestricted(setupCtx), database.UpdateUserGithubComUserIDParams{
				ID: userData.ID,
				GithubComUserID: sql.NullInt64{
					Int64: int64(i),
					Valid: true,
				},
			})
			require.NoError(t, err)
		}

		user, err := userClient.User(setupCtx, codersdk.Me)
		require.NoError(t, err, "fetch me")

		if i%4 == 0 {
			user, err = client.UpdateUserStatus(setupCtx, user.ID.String(), codersdk.UserStatusSuspended)
			require.NoError(t, err, "suspend user")
		}

		if i%5 == 0 {
			user, err = client.UpdateUserProfile(setupCtx, user.ID.String(), codersdk.UpdateUserProfileRequest{
				Username: strings.ToUpper(user.Username),
			})
			require.NoError(t, err, "update username to uppercase")
		}

		users = append(users, user)
	}

	// Add some service accounts.
	if options.CreateServiceAccounts {
		for range 3 {
			_, user := CreateAnotherUserMutators(t, client, orgID, nil, func(r *codersdk.CreateUserRequestWithOrgs) {
				r.ServiceAccount = true
			})
			users = append(users, user)
		}
	}

	hashedPassword, err := userpassword.Hash("SomeStrongPassword!")
	require.NoError(t, err)

	// Add users with different creation dates for testing date filters
	for i := range 3 {
		// nolint:gocritic // Setting up unit test data.
		user1, err := db.InsertUser(dbauthz.AsSystemRestricted(setupCtx), database.InsertUserParams{
			ID:               uuid.New(),
			Email:            fmt.Sprintf("before%d@coder.com", i),
			Username:         fmt.Sprintf("before%d", i),
			Name:             fmt.Sprintf("Test User %d", i),
			HashedPassword:   []byte(hashedPassword),
			LoginType:        database.LoginTypeNone,
			Status:           string(codersdk.UserStatusActive),
			RBACRoles:        []string{codersdk.RoleMember},
			CreatedAt:        dbtime.Time(time.Date(2022, 12, 15+i, 12, 0, 0, 0, time.UTC)),
			UpdatedAt:        dbtime.Time(time.Date(2022, 12, 15+i, 12, 0, 0, 0, time.UTC)),
			IsServiceAccount: false,
		})
		require.NoError(t, err)
		// nolint:gocritic // Setting up unit test data.
		_, err = db.InsertOrganizationMember(dbauthz.AsSystemRestricted(setupCtx), database.InsertOrganizationMemberParams{
			OrganizationID: orgID,
			UserID:         user1.ID,
			CreatedAt:      dbtime.Now(),
			UpdatedAt:      dbtime.Now(),
			Roles:          []string{},
		})
		require.NoError(t, err)

		// The expected timestamps must be parsed from strings to compare equal during `ElementsMatch`
		sdkUser1 := db2sdk.User(user1, []uuid.UUID{orgID})
		sdkUser1.CreatedAt, err = time.Parse(time.RFC3339, sdkUser1.CreatedAt.Format(time.RFC3339))
		require.NoError(t, err)
		sdkUser1.UpdatedAt, err = time.Parse(time.RFC3339, sdkUser1.UpdatedAt.Format(time.RFC3339))
		require.NoError(t, err)
		sdkUser1.LastSeenAt, err = time.Parse(time.RFC3339, sdkUser1.LastSeenAt.Format(time.RFC3339))
		require.NoError(t, err)
		users = append(users, sdkUser1)

		// nolint:gocritic // Setting up unit test data.
		user2, err := db.InsertUser(dbauthz.AsSystemRestricted(setupCtx), database.InsertUserParams{
			ID:               uuid.New(),
			Email:            fmt.Sprintf("during%d@coder.com", i),
			Username:         fmt.Sprintf("during%d", i),
			Name:             "",
			HashedPassword:   []byte(hashedPassword),
			LoginType:        database.LoginTypeNone,
			Status:           string(codersdk.UserStatusActive),
			RBACRoles:        []string{codersdk.RoleOwner},
			CreatedAt:        dbtime.Time(time.Date(2023, 1, 15+i, 12, 0, 0, 0, time.UTC)),
			UpdatedAt:        dbtime.Time(time.Date(2023, 1, 15+i, 12, 0, 0, 0, time.UTC)),
			IsServiceAccount: false,
		})
		require.NoError(t, err)
		// nolint:gocritic // Setting up unit test data.
		_, err = db.InsertOrganizationMember(dbauthz.AsSystemRestricted(setupCtx), database.InsertOrganizationMemberParams{
			OrganizationID: orgID,
			UserID:         user2.ID,
			CreatedAt:      dbtime.Now(),
			UpdatedAt:      dbtime.Now(),
			Roles:          []string{},
		})
		require.NoError(t, err)

		sdkUser2 := db2sdk.User(user2, []uuid.UUID{orgID})
		sdkUser2.CreatedAt, err = time.Parse(time.RFC3339, sdkUser2.CreatedAt.Format(time.RFC3339))
		require.NoError(t, err)
		sdkUser2.UpdatedAt, err = time.Parse(time.RFC3339, sdkUser2.UpdatedAt.Format(time.RFC3339))
		require.NoError(t, err)
		sdkUser2.LastSeenAt, err = time.Parse(time.RFC3339, sdkUser2.LastSeenAt.Format(time.RFC3339))
		require.NoError(t, err)
		users = append(users, sdkUser2)

		// nolint:gocritic // Setting up unit test data.
		user3, err := db.InsertUser(dbauthz.AsSystemRestricted(setupCtx), database.InsertUserParams{
			ID:               uuid.New(),
			Email:            fmt.Sprintf("after%d@coder.com", i),
			Username:         fmt.Sprintf("after%d", i),
			Name:             "",
			HashedPassword:   []byte(hashedPassword),
			LoginType:        database.LoginTypeNone,
			Status:           string(codersdk.UserStatusActive),
			RBACRoles:        []string{codersdk.RoleOwner},
			CreatedAt:        dbtime.Time(time.Date(2023, 2, 15+i, 12, 0, 0, 0, time.UTC)),
			UpdatedAt:        dbtime.Time(time.Date(2023, 2, 15+i, 12, 0, 0, 0, time.UTC)),
			IsServiceAccount: false,
		})
		require.NoError(t, err)
		// nolint:gocritic // Setting up unit test data.
		_, err = db.InsertOrganizationMember(dbauthz.AsSystemRestricted(setupCtx), database.InsertOrganizationMemberParams{
			OrganizationID: orgID,
			UserID:         user3.ID,
			CreatedAt:      dbtime.Now(),
			UpdatedAt:      dbtime.Now(),
			Roles:          []string{},
		})
		require.NoError(t, err)

		sdkUser3 := db2sdk.User(user3, []uuid.UUID{orgID})
		sdkUser3.CreatedAt, err = time.Parse(time.RFC3339, sdkUser3.CreatedAt.Format(time.RFC3339))
		require.NoError(t, err)
		sdkUser3.UpdatedAt, err = time.Parse(time.RFC3339, sdkUser3.UpdatedAt.Format(time.RFC3339))
		require.NoError(t, err)
		sdkUser3.LastSeenAt, err = time.Parse(time.RFC3339, sdkUser3.LastSeenAt.Format(time.RFC3339))
		require.NoError(t, err)
		users = append(users, sdkUser3)
	}

	if setup != nil {
		setup(users)
	}

	// --- Setup done ---
	testCases := []struct {
		Name   string
		Filter codersdk.UsersRequest
		// If FilterF is true, we include it in the expected results
		FilterF func(f codersdk.UsersRequest, user codersdk.User) bool
	}{
		{
			Name: "All",
			Filter: codersdk.UsersRequest{
				Status: codersdk.UserStatusSuspended + "," + codersdk.UserStatusActive,
			},
			FilterF: func(_ codersdk.UsersRequest, _ codersdk.User) bool {
				return true
			},
		},
		{
			Name: "Active",
			Filter: codersdk.UsersRequest{
				Status: codersdk.UserStatusActive,
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				return u.Status == codersdk.UserStatusActive
			},
		},
		{
			Name: "GithubComUserID",
			Filter: codersdk.UsersRequest{
				SearchQuery: "github_com_user_id:7",
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				return strings.HasSuffix(u.Username, "-gh7")
			},
		},
		{
			Name: "ActiveUppercase",
			Filter: codersdk.UsersRequest{
				Status: "ACTIVE",
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				return u.Status == codersdk.UserStatusActive
			},
		},
		{
			Name: "Suspended",
			Filter: codersdk.UsersRequest{
				Status: codersdk.UserStatusSuspended,
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				return u.Status == codersdk.UserStatusSuspended
			},
		},
		{
			Name: "NameContains",
			Filter: codersdk.UsersRequest{
				Search: "a",
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				return (strings.ContainsAny(u.Username, "aA") || strings.ContainsAny(u.Email, "aA"))
			},
		},
		{
			Name: "NameAndSearch",
			Filter: codersdk.UsersRequest{
				SearchQuery: "name:Test search:before1",
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				return u.Username == "before1"
			},
		},
		{
			Name: "NameNoMatch",
			Filter: codersdk.UsersRequest{
				Search: "nonexistent",
			},
			FilterF: func(_ codersdk.UsersRequest, _ codersdk.User) bool {
				return false
			},
		},
		{
			Name: "Admins",
			Filter: codersdk.UsersRequest{
				Role:   codersdk.RoleOwner,
				Status: codersdk.UserStatusSuspended + "," + codersdk.UserStatusActive,
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				for _, r := range u.Roles {
					if r.Name == codersdk.RoleOwner {
						return true
					}
				}
				return false
			},
		},
		{
			Name: "AdminsUppercase",
			Filter: codersdk.UsersRequest{
				Role:   "OWNER",
				Status: codersdk.UserStatusSuspended + "," + codersdk.UserStatusActive,
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				for _, r := range u.Roles {
					if r.Name == codersdk.RoleOwner {
						return true
					}
				}
				return false
			},
		},
		{
			Name: "Members",
			Filter: codersdk.UsersRequest{
				Role:   codersdk.RoleMember,
				Status: codersdk.UserStatusSuspended + "," + codersdk.UserStatusActive,
			},
			FilterF: func(_ codersdk.UsersRequest, _ codersdk.User) bool {
				return true
			},
		},
		{
			Name: "SearchQuery",
			Filter: codersdk.UsersRequest{
				SearchQuery: "i role:owner status:active",
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				for _, r := range u.Roles {
					if r.Name == codersdk.RoleOwner {
						return (strings.ContainsAny(u.Username, "iI") || strings.ContainsAny(u.Email, "iI")) &&
							u.Status == codersdk.UserStatusActive
					}
				}
				return false
			},
		},
		{
			Name: "SearchQueryInsensitive",
			Filter: codersdk.UsersRequest{
				SearchQuery: "i Role:Owner STATUS:Active",
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				for _, r := range u.Roles {
					if r.Name == codersdk.RoleOwner {
						return (strings.ContainsAny(u.Username, "iI") || strings.ContainsAny(u.Email, "iI")) &&
							u.Status == codersdk.UserStatusActive
					}
				}
				return false
			},
		},
		{
			Name: "LastSeenBeforeNow",
			Filter: codersdk.UsersRequest{
				SearchQuery: `last_seen_before:"2023-01-16T00:00:00+12:00"`,
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				return u.LastSeenAt.Before(lastSeenNow)
			},
		},
		{
			Name: "LastSeenLastWeek",
			Filter: codersdk.UsersRequest{
				SearchQuery: `last_seen_before:"2023-01-14T23:59:59Z" last_seen_after:"2023-01-08T00:00:00Z"`,
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				start := time.Date(2023, 1, 8, 0, 0, 0, 0, time.UTC)
				end := time.Date(2023, 1, 14, 23, 59, 59, 0, time.UTC)
				return u.LastSeenAt.Before(end) && u.LastSeenAt.After(start)
			},
		},
		{
			Name: "CreatedAtBefore",
			Filter: codersdk.UsersRequest{
				SearchQuery: `created_before:"2023-01-31T23:59:59Z"`,
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				end := time.Date(2023, 1, 31, 23, 59, 59, 0, time.UTC)
				return u.CreatedAt.Before(end)
			},
		},
		{
			Name: "CreatedAtAfter",
			Filter: codersdk.UsersRequest{
				SearchQuery: `created_after:"2023-01-01T00:00:00Z"`,
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				start := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
				return u.CreatedAt.After(start)
			},
		},
		{
			Name: "CreatedAtRange",
			Filter: codersdk.UsersRequest{
				SearchQuery: `created_after:"2023-01-01T00:00:00Z" created_before:"2023-01-31T23:59:59Z"`,
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				start := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
				end := time.Date(2023, 1, 31, 23, 59, 59, 0, time.UTC)
				return u.CreatedAt.After(start) && u.CreatedAt.Before(end)
			},
		},
		{
			Name: "LoginTypeNone",
			Filter: codersdk.UsersRequest{
				LoginType: []codersdk.LoginType{codersdk.LoginTypeNone},
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				return u.LoginType == codersdk.LoginTypeNone
			},
		},
		{
			Name: "LoginTypeOIDC",
			Filter: codersdk.UsersRequest{
				LoginType: []codersdk.LoginType{codersdk.LoginTypeOIDC},
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				return u.LoginType == codersdk.LoginTypeOIDC
			},
		},
		{
			Name: "LoginTypeMultiple",
			Filter: codersdk.UsersRequest{
				LoginType: []codersdk.LoginType{codersdk.LoginTypeNone, codersdk.LoginTypeGithub},
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				return u.LoginType == codersdk.LoginTypeNone || u.LoginType == codersdk.LoginTypeGithub
			},
		},
		{
			Name: "DormantUserWithLoginTypeNone",
			Filter: codersdk.UsersRequest{
				Status:    codersdk.UserStatusSuspended,
				LoginType: []codersdk.LoginType{codersdk.LoginTypeNone},
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				return u.Status == codersdk.UserStatusSuspended && u.LoginType == codersdk.LoginTypeNone
			},
		},
		{
			Name: "IsServiceAccount",
			Filter: codersdk.UsersRequest{
				Search: "service_account:true",
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				return u.IsServiceAccount
			},
		},
		{
			Name: "IsNotServiceAccount",
			Filter: codersdk.UsersRequest{
				Search: "service_account:false",
			},
			FilterF: func(_ codersdk.UsersRequest, u codersdk.User) bool {
				return !u.IsServiceAccount
			},
		},
	}

	for _, c := range testCases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			testCtx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			got := fetch(testCtx, c.Filter)
			exp := make([]codersdk.ReducedUser, 0)
			for _, made := range users {
				match := c.FilterF(c.Filter, made)
				if match {
					exp = append(exp, made.ReducedUser)
				}
			}

			require.ElementsMatch(t, exp, got, "expected users returned")
		})
	}
}
