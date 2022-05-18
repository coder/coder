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
				roles := []string{rbac.RoleMember()}
				user, token := addUser(t, db, roles...)
				return user, roles, token
			},
		},
		{
			Name: "Admin",
			AddUser: func(db database.Store) (database.User, []string, string) {
				roles := []string{rbac.RoleMember(), rbac.RoleAdmin()}
				user, token := addUser(t, db, roles...)
				return user, roles, token
			},
		},
		{
			Name: "OrgMember",
			AddUser: func(db database.Store) (database.User, []string, string) {
				roles := []string{rbac.RoleMember()}
				user, token := addUser(t, db, roles...)
				org, err := db.InsertOrganization(context.Background(), database.InsertOrganizationParams{
					ID:          uuid.New(),
					Name:        "testorg",
					Description: "test",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				})
				require.NoError(t, err)

				orgRoles := []string{rbac.RoleOrgMember(org.ID)}
				_, err = db.InsertOrganizationMember(context.Background(), database.InsertOrganizationMemberParams{
					OrganizationID: org.ID,
					UserID:         user.ID,
					CreatedAt:      time.Now(),
					UpdatedAt:      time.Now(),
					Roles:          orgRoles,
				})
				require.NoError(t, err)
				return user, append(roles, orgRoles...), token
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
				httpmw.ExtractAPIKey(db, &httpmw.OAuth2Configs{}),
				httpmw.ExtractUserRoles(db),
			)
			rtr.Get("/", func(_ http.ResponseWriter, r *http.Request) {
				roles := httpmw.UserRoles(r)
				require.ElementsMatch(t, user.ID, roles.ID)
				require.ElementsMatch(t, expRoles, roles.Roles)
			})

			req := httptest.NewRequest("GET", "/", nil)
			req.AddCookie(&http.Cookie{
				Name:  httpmw.SessionTokenKey,
				Value: token,
			})

			rtr.ServeHTTP(rw, req)
			require.Equal(t, http.StatusOK, rw.Result().StatusCode)
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
	})
	require.NoError(t, err)

	return user, fmt.Sprintf("%s-%s", id, secret)
}
