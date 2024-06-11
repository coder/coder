package httpmw_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestOrganizationParam(t *testing.T) {
	t.Parallel()

	setupAuthentication := func(db database.Store) (*http.Request, database.User) {
		r := httptest.NewRequest("GET", "/", nil)

		user := dbgen.User(t, db, database.User{
			ID: uuid.New(),
		})
		_, token := dbgen.APIKey(t, db, database.APIKey{
			UserID: user.ID,
		})
		r.Header.Set(codersdk.SessionTokenHeader, token)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, chi.NewRouteContext()))
		return r, user
	}

	t.Run("None", func(t *testing.T) {
		t.Parallel()
		var (
			db   = dbmem.New()
			rw   = httptest.NewRecorder()
			r, _ = setupAuthentication(db)
			rtr  = chi.NewRouter()
		)
		rtr.Use(
			httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
				DB:              db,
				RedirectToLogin: false,
			}),
			httpmw.ExtractOrganizationParam(db),
		)
		rtr.Get("/", nil)
		rtr.ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		var (
			db   = dbmem.New()
			rw   = httptest.NewRecorder()
			r, _ = setupAuthentication(db)
			rtr  = chi.NewRouter()
		)
		chi.RouteContext(r.Context()).URLParams.Add("organization", uuid.NewString())
		rtr.Use(
			httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
				DB:              db,
				RedirectToLogin: false,
			}),
			httpmw.ExtractOrganizationParam(db),
		)
		rtr.Get("/", nil)
		rtr.ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("InvalidUUID", func(t *testing.T) {
		t.Parallel()
		var (
			db   = dbmem.New()
			rw   = httptest.NewRecorder()
			r, _ = setupAuthentication(db)
			rtr  = chi.NewRouter()
		)
		chi.RouteContext(r.Context()).URLParams.Add("organization", "not-a-uuid")
		rtr.Use(
			httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
				DB:              db,
				RedirectToLogin: false,
			}),
			httpmw.ExtractOrganizationParam(db),
		)
		rtr.Get("/", nil)
		rtr.ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("NotInOrganization", func(t *testing.T) {
		t.Parallel()
		var (
			db   = dbmem.New()
			rw   = httptest.NewRecorder()
			r, u = setupAuthentication(db)
			rtr  = chi.NewRouter()
		)
		organization, err := db.InsertOrganization(r.Context(), database.InsertOrganizationParams{
			ID:        uuid.New(),
			Name:      "test",
			CreatedAt: dbtime.Now(),
			UpdatedAt: dbtime.Now(),
		})
		require.NoError(t, err)
		chi.RouteContext(r.Context()).URLParams.Add("organization", organization.ID.String())
		chi.RouteContext(r.Context()).URLParams.Add("user", u.ID.String())
		rtr.Use(
			httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
				DB:              db,
				RedirectToLogin: false,
			}),
			httpmw.ExtractUserParam(db),
			httpmw.ExtractOrganizationParam(db),
			httpmw.ExtractOrganizationMemberParam(db),
		)
		rtr.Get("/", nil)
		rtr.ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		var (
			ctx     = testutil.Context(t, testutil.WaitShort)
			db      = dbmem.New()
			rw      = httptest.NewRecorder()
			r, user = setupAuthentication(db)
			rtr     = chi.NewRouter()
		)
		organization := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			OrganizationID: organization.ID,
			UserID:         user.ID,
			Roles:          []string{codersdk.RoleOrganizationMember},
		})
		_, err := db.UpdateUserRoles(ctx, database.UpdateUserRolesParams{
			ID:           user.ID,
			GrantedRoles: []string{codersdk.RoleTemplateAdmin},
		})
		require.NoError(t, err)

		rtr.Use(
			httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
				DB:              db,
				RedirectToLogin: false,
			}),
			httpmw.ExtractOrganizationParam(db),
			httpmw.ExtractUserParam(db),
			httpmw.ExtractOrganizationMemberParam(db),
		)
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			org := httpmw.OrganizationParam(r)
			assert.NotZero(t, org)
			assert.NotZero(t, org.CreatedAt)
			// assert.NotZero(t, org.Description) // not supported
			assert.NotZero(t, org.ID)
			assert.NotEmpty(t, org.Name)
			orgMem := httpmw.OrganizationMemberParam(r)
			rw.WriteHeader(http.StatusOK)
			assert.NotZero(t, orgMem)
			assert.NotZero(t, orgMem.CreatedAt)
			assert.NotZero(t, orgMem.UpdatedAt)
			assert.Equal(t, org.ID, orgMem.OrganizationID)
			assert.Equal(t, user.ID, orgMem.UserID)
			assert.Equal(t, user.Username, orgMem.Username)
			assert.Equal(t, user.AvatarURL, orgMem.AvatarURL)
			assert.NotEmpty(t, orgMem.Roles)
			assert.NotZero(t, orgMem.OrganizationMember)
			assert.NotEmpty(t, orgMem.OrganizationMember.CreatedAt)
			assert.NotEmpty(t, orgMem.OrganizationMember.UpdatedAt)
			assert.NotEmpty(t, orgMem.OrganizationMember.UserID)
			assert.NotEmpty(t, orgMem.OrganizationMember.Roles)
		})

		// Try by ID
		chi.RouteContext(r.Context()).URLParams.Add("organization", organization.ID.String())
		chi.RouteContext(r.Context()).URLParams.Add("user", user.ID.String())
		rtr.ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode, "by id")

		// Try by name
		chi.RouteContext(r.Context()).URLParams.Add("organization", organization.Name)
		chi.RouteContext(r.Context()).URLParams.Add("user", user.ID.String())
		rtr.ServeHTTP(rw, r)
		res = rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode, "by name")

		// Try by 'default'
		chi.RouteContext(r.Context()).URLParams.Add("organization", codersdk.DefaultOrganization)
		chi.RouteContext(r.Context()).URLParams.Add("user", user.ID.String())
		rtr.ServeHTTP(rw, r)
		res = rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode, "by default keyword")

		// Try by legacy
		// TODO: This can be removed when legacy nil uuids are no longer supported.
		//		 This is a temporary measure to ensure as legacy provisioners use
		//		 nil uuids as the org id and expect the default org.
		chi.RouteContext(r.Context()).URLParams.Add("organization", uuid.Nil.String())
		chi.RouteContext(r.Context()).URLParams.Add("user", user.ID.String())
		rtr.ServeHTTP(rw, r)
		res = rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode, "by nil uuid (legacy)")
	})
}
