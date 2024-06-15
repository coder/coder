package httpmw_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

func TestExtractUserRoles(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name    string
		AddUser func(db database.Store) (database.User, []rbac.RoleIdentifier, string)
	}{
		{
			Name: "Member",
			AddUser: func(db database.Store) (database.User, []rbac.RoleIdentifier, string) {
				user, token := addUser(t, db)
				return user, []rbac.RoleIdentifier{rbac.RoleMember()}, token
			},
		},
		{
			Name: "Owner",
			AddUser: func(db database.Store) (database.User, []rbac.RoleIdentifier, string) {
				roles := []string{codersdk.RoleOwner}
				user, token := addUser(t, db, roles...)
				return user, []rbac.RoleIdentifier{rbac.RoleOwner(), rbac.RoleMember()}, token
			},
		},
		{
			Name: "OrgMember",
			AddUser: func(db database.Store) (database.User, []rbac.RoleIdentifier, string) {
				roles := []string{}
				user, token := addUser(t, db, roles...)
				org, err := db.InsertOrganization(context.Background(), database.InsertOrganizationParams{
					ID:          uuid.New(),
					Name:        "testorg",
					Description: "test",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				})
				require.NoError(t, err)

				orgRoles := []string{}
				_, err = db.InsertOrganizationMember(context.Background(), database.InsertOrganizationMemberParams{
					OrganizationID: org.ID,
					UserID:         user.ID,
					CreatedAt:      time.Now(),
					UpdatedAt:      time.Now(),
					Roles:          orgRoles,
				})
				require.NoError(t, err)
				return user, []rbac.RoleIdentifier{rbac.RoleMember(), rbac.ScopedRoleOrgMember(org.ID)}, token
			},
		},
		{
			Name: "MultipleOrgMember",
			AddUser: func(db database.Store) (database.User, []rbac.RoleIdentifier, string) {
				expected := []rbac.RoleIdentifier{}
				user, token := addUser(t, db)
				expected = append(expected, rbac.RoleMember())
				for i := 0; i < 3; i++ {
					organization, err := db.InsertOrganization(context.Background(), database.InsertOrganizationParams{
						ID:          uuid.New(),
						Name:        fmt.Sprintf("testorg%d", i),
						Description: "test",
						CreatedAt:   time.Now(),
						UpdatedAt:   time.Now(),
					})
					require.NoError(t, err)

					orgRoles := []string{}
					if i%2 == 0 {
						orgRoles = append(orgRoles, codersdk.RoleOrganizationAdmin)
						expected = append(expected, rbac.ScopedRoleOrgAdmin(organization.ID))
					}
					_, err = db.InsertOrganizationMember(context.Background(), database.InsertOrganizationMemberParams{
						OrganizationID: organization.ID,
						UserID:         user.ID,
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
						Roles:          orgRoles,
					})
					require.NoError(t, err)
					expected = append(expected, rbac.ScopedRoleOrgMember(organization.ID))
				}
				return user, expected, token
			},
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			var (
				db, _                 = dbtestutil.NewDB(t)
				user, expRoles, token = c.AddUser(db)
				rw                    = httptest.NewRecorder()
				rtr                   = chi.NewRouter()
			)
			rtr.Use(
				httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
					DB:              db,
					OAuth2Configs:   &httpmw.OAuth2Configs{},
					RedirectToLogin: false,
				}),
			)
			rtr.Get("/", func(_ http.ResponseWriter, r *http.Request) {
				roles := httpmw.UserAuthorization(r)
				require.Equal(t, user.ID.String(), roles.ID)
				require.ElementsMatch(t, expRoles, roles.Roles.Names())
			})

			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set(codersdk.SessionTokenHeader, token)

			rtr.ServeHTTP(rw, req)
			resp := rw.Result()
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}

func addUser(t *testing.T, db database.Store, roles ...string) (database.User, string) {
	var (
		id, secret = randomAPIKeyParts()
		hashed     = sha256.Sum256([]byte(secret))
	)
	if roles == nil {
		roles = []string{}
	}

	user, err := db.InsertUser(context.Background(), database.InsertUserParams{
		ID:        uuid.New(),
		Email:     "admin@email.com",
		Username:  "admin",
		RBACRoles: roles,
		LoginType: database.LoginTypePassword,
	})
	require.NoError(t, err)

	user, err = db.UpdateUserStatus(context.Background(), database.UpdateUserStatusParams{
		ID:        user.ID,
		Status:    database.UserStatusActive,
		UpdatedAt: dbtime.Now(),
	})
	require.NoError(t, err)

	_, err = db.InsertAPIKey(context.Background(), database.InsertAPIKeyParams{
		ID:           id,
		UserID:       user.ID,
		HashedSecret: hashed[:],
		LastUsed:     dbtime.Now(),
		ExpiresAt:    dbtime.Now().Add(time.Minute),
		LoginType:    database.LoginTypePassword,
		Scope:        database.APIKeyScopeAll,
		IPAddress: pqtype.Inet{
			IPNet: net.IPNet{
				IP:   net.ParseIP("0.0.0.0"),
				Mask: net.IPMask{0, 0, 0, 0},
			},
			Valid: true,
		},
	})
	require.NoError(t, err)

	return user, fmt.Sprintf("%s-%s", id, secret)
}
