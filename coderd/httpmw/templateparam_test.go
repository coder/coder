package httpmw_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

func TestTemplateParam(t *testing.T) {
	t.Parallel()

	setupAuthentication := func(db database.Store) (*http.Request, database.Organization) {
		var (
			user     = dbgen.User(t, db, database.User{})
			_, token = dbgen.APIKey(t, db, database.APIKey{
				UserID: user.ID,
			})
			organization = dbgen.Organization(t, db, database.Organization{})
			_            = dbgen.OrganizationMember(t, db, database.OrganizationMember{
				UserID:         user.ID,
				OrganizationID: organization.ID,
			})
		)

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		ctx := chi.NewRouteContext()
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
		return r, organization
	}

	t.Run("None", func(t *testing.T) {
		t.Parallel()
		db := dbmem.New()
		rtr := chi.NewRouter()
		rtr.Use(httpmw.ExtractTemplateParam(db))
		rtr.Get("/", nil)
		r, _ := setupAuthentication(db)
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		db := dbmem.New()
		rtr := chi.NewRouter()
		rtr.Use(httpmw.ExtractTemplateParam(db))
		rtr.Get("/", nil)

		r, _ := setupAuthentication(db)
		chi.RouteContext(r.Context()).URLParams.Add("template", uuid.NewString())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("BadUUID", func(t *testing.T) {
		t.Parallel()
		db := dbmem.New()
		rtr := chi.NewRouter()
		rtr.Use(httpmw.ExtractTemplateParam(db))
		rtr.Get("/", nil)

		r, _ := setupAuthentication(db)
		chi.RouteContext(r.Context()).URLParams.Add("template", "not-a-uuid")
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("Template", func(t *testing.T) {
		t.Parallel()
		db := dbmem.New()
		rtr := chi.NewRouter()
		rtr.Use(
			httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
				DB:              db,
				RedirectToLogin: false,
			}),
			httpmw.ExtractTemplateParam(db),
			httpmw.ExtractOrganizationParam(db),
		)
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			_ = httpmw.TemplateParam(r)
			rw.WriteHeader(http.StatusOK)
		})

		r, org := setupAuthentication(db)
		template := dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			Provisioner:    database.ProvisionerTypeEcho,
		})

		chi.RouteContext(r.Context()).URLParams.Add("template", template.ID.String())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})
}
