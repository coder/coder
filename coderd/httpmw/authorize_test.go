package httpmw_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/coderd/httpmw"
)

func TestExtractUserRoles(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name    string
		AddUser func(db database.Store) (database.User, []string, string)
	}{
		{
			Name: "Member",
			AddUser: func(db database.Store) (database.User, []string, string) {
				roles := []string{}
				user, token := addUser(t, db, roles...)
				return user, append(roles, rbac.RoleMember()), token
			},
		},
		{
			Name: "Admin",
			AddUser: func(db database.Store) (database.User, []string, string) {
				roles := []string{rbac.RoleOwner()}
				user, token := addUser(t, db, roles...)
				return user, append(roles, rbac.RoleMember()), token
			},
		},
		{
			Name: "OrgMember",
			AddUser: func(db database.Store) (database.User, []string, string) {
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
				return user, append(roles, append(orgRoles, rbac.RoleMember(), rbac.RoleOrgMember(org.ID))...), token
			},
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			var (
				db                    = databasefake.New()
				user, expRoles, token = c.AddUser(db)
				rw                    = httptest.NewRecorder()
				rtr                   = chi.NewRouter()
			)
			rtr.Use(
				httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
					DB:              db,
					OAuth2Configs:   &httpmw.OAuth2Configs{},
					RedirectToLogin: false,
				}),
			)
			rtr.Get("/", func(_ http.ResponseWriter, r *http.Request) {
				roles := httpmw.UserAuthorization(r)
				require.ElementsMatch(t, user.ID, roles.ID)
				require.ElementsMatch(t, expRoles, roles.Roles)
			})

			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set(codersdk.SessionCustomHeader, token)

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

	user, err := db.InsertUser(context.Background(), database.InsertUserParams{
		ID:        uuid.New(),
		Email:     "admin@email.com",
		Username:  "admin",
		RBACRoles: roles,
	})
	require.NoError(t, err)

	_, err = db.InsertAPIKey(context.Background(), database.InsertAPIKeyParams{
		ID:           id,
		UserID:       user.ID,
		HashedSecret: hashed[:],
		LastUsed:     database.Now(),
		ExpiresAt:    database.Now().Add(time.Minute),
		LoginType:    database.LoginTypePassword,
		Scope:        database.APIKeyScopeAll,
	})
	require.NoError(t, err)

	return user, fmt.Sprintf("%s-%s", id, secret)
}
