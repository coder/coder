package httpmw_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
)

func TestTemplateVersionParam(t *testing.T) {
	t.Parallel()

	setupAuthentication := func(db database.Store) (*http.Request, database.Template) {
		var (
			id, secret = randomAPIKeyParts()
			hashed     = sha256.Sum256([]byte(secret))
		)
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(codersdk.SessionCustomHeader, fmt.Sprintf("%s-%s", id, secret))

		userID := uuid.New()
		username, err := cryptorand.String(8)
		require.NoError(t, err)
		user, err := db.InsertUser(r.Context(), database.InsertUserParams{
			ID:             userID,
			Email:          "testaccount@coder.com",
			HashedPassword: hashed[:],
			Username:       username,
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
			LoginType:      database.LoginTypePassword,
		})
		require.NoError(t, err)

		_, err = db.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
			ID:           id,
			UserID:       user.ID,
			HashedSecret: hashed[:],
			LastUsed:     database.Now(),
			ExpiresAt:    database.Now().Add(time.Minute),
			LoginType:    database.LoginTypePassword,
			Scope:        database.APIKeyScopeAll,
		})
		require.NoError(t, err)

		orgID := uuid.New()
		organization, err := db.InsertOrganization(r.Context(), database.InsertOrganizationParams{
			ID:          orgID,
			Name:        "banana",
			Description: "wowie",
			CreatedAt:   database.Now(),
			UpdatedAt:   database.Now(),
		})
		require.NoError(t, err)

		_, err = db.InsertOrganizationMember(r.Context(), database.InsertOrganizationMemberParams{
			OrganizationID: orgID,
			UserID:         user.ID,
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
		})
		require.NoError(t, err)

		template, err := db.InsertTemplate(context.Background(), database.InsertTemplateParams{
			ID:             uuid.New(),
			OrganizationID: organization.ID,
			Name:           "moo",
			Provisioner:    database.ProvisionerTypeEcho,
		})
		require.NoError(t, err)

		ctx := chi.NewRouteContext()
		ctx.URLParams.Add("organization", organization.Name)
		ctx.URLParams.Add("template", template.Name)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
		return r, template
	}

	t.Run("None", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		rtr := chi.NewRouter()
		rtr.Use(httpmw.ExtractTemplateVersionParam(db))
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
		db := databasefake.New()
		rtr := chi.NewRouter()
		rtr.Use(httpmw.ExtractTemplateVersionParam(db))
		rtr.Get("/", nil)

		r, _ := setupAuthentication(db)
		chi.RouteContext(r.Context()).URLParams.Add("templateversion", uuid.NewString())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("TemplateVersion", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		rtr := chi.NewRouter()
		rtr.Use(
			httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
				DB:              db,
				RedirectToLogin: false,
			}),
			httpmw.ExtractTemplateVersionParam(db),
			httpmw.ExtractOrganizationParam(db),
		)
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			_ = httpmw.TemplateVersionParam(r)
			rw.WriteHeader(http.StatusOK)
		})

		r, template := setupAuthentication(db)
		templateVersion, err := db.InsertTemplateVersion(context.Background(), database.InsertTemplateVersionParams{
			ID:             uuid.New(),
			OrganizationID: template.OrganizationID,
			Name:           "moo",
		})
		require.NoError(t, err)
		chi.RouteContext(r.Context()).URLParams.Add("templateversion", templateVersion.ID.String())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})
}
