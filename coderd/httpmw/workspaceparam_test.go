package httpmw_test

import (
	"context"
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
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
)

func TestWorkspaceParam(t *testing.T) {
	t.Parallel()

	setup := func(db database.Store) (*http.Request, database.User) {
		id, secret, hashed := randomAPIKeyParts()
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(codersdk.SessionTokenHeader, fmt.Sprintf("%s-%s", id, secret))

		userID := uuid.New()
		username, err := cryptorand.String(8)
		require.NoError(t, err)
		user, err := db.InsertUser(r.Context(), database.InsertUserParams{
			ID:             userID,
			Email:          "testaccount@coder.com",
			HashedPassword: hashed,
			Username:       username,
			CreatedAt:      dbtime.Now(),
			UpdatedAt:      dbtime.Now(),
			LoginType:      database.LoginTypePassword,
			RBACRoles:      []string{},
		})
		require.NoError(t, err)

		user, err = db.UpdateUserStatus(context.Background(), database.UpdateUserStatusParams{
			ID:        user.ID,
			Status:    database.UserStatusActive,
			UpdatedAt: dbtime.Now(),
		})
		require.NoError(t, err)

		_, err = db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			UserID:       user.ID,
			HashedSecret: hashed,
			LastUsed:     dbtime.Now(),
			ExpiresAt:    dbtime.Now().Add(time.Minute),
			LoginType:    database.LoginTypePassword,
			Scopes:       database.APIKeyScopes{database.ApiKeyScopeCoderAll},
			AllowList: database.AllowList{
				{Type: policy.WildcardSymbol, ID: policy.WildcardSymbol},
			},
			IPAddress: pqtype.Inet{
				IPNet: net.IPNet{
					IP:   net.IPv4(127, 0, 0, 1),
					Mask: net.IPv4Mask(255, 255, 255, 255),
				},
				Valid: true,
			},
		})
		require.NoError(t, err)

		ctx := chi.NewRouteContext()
		ctx.URLParams.Add("user", "me")
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
		return r, user
	}

	t.Run("None", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		rtr := chi.NewRouter()
		rtr.Use(httpmw.ExtractWorkspaceParam(db))
		rtr.Get("/", nil)
		r, _ := setup(db)
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		rtr := chi.NewRouter()
		rtr.Use(httpmw.ExtractWorkspaceParam(db))
		rtr.Get("/", nil)
		r, _ := setup(db)
		chi.RouteContext(r.Context()).URLParams.Add("workspace", uuid.NewString())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		rtr := chi.NewRouter()
		rtr.Use(
			httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
				DB:              db,
				RedirectToLogin: false,
			}),
			httpmw.ExtractWorkspaceParam(db),
		)
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			_ = httpmw.WorkspaceParam(r)
			rw.WriteHeader(http.StatusOK)
		})
		r, user := setup(db)
		org := dbgen.Organization(t, db, database.Organization{})
		tpl := dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		})
		workspace, err := db.InsertWorkspace(context.Background(), database.InsertWorkspaceParams{
			ID:               uuid.New(),
			OwnerID:          user.ID,
			Name:             "hello",
			AutomaticUpdates: database.AutomaticUpdatesNever,
			OrganizationID:   org.ID,
			TemplateID:       tpl.ID,
		})
		require.NoError(t, err)
		chi.RouteContext(r.Context()).URLParams.Add("workspace", workspace.ID.String())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})
}
