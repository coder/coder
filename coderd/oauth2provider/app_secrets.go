package oauth2provider

import (
	"net/http"

	"github.com/google/uuid"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// GetAppSecrets returns an http.HandlerFunc that handles GET /oauth2-provider/apps/{app}/secrets
func GetAppSecrets(db database.Store) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		app := httpmw.OAuth2ProviderApp(r)
		dbSecrets, err := db.GetOAuth2ProviderAppSecretsByAppID(ctx, app.ID)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error getting OAuth2 client secrets.",
				Detail:  err.Error(),
			})
			return
		}
		secrets := []codersdk.OAuth2ProviderAppSecret{}
		for _, secret := range dbSecrets {
			secrets = append(secrets, codersdk.OAuth2ProviderAppSecret{
				ID:                    secret.ID,
				LastUsedAt:            codersdk.NullTime{NullTime: secret.LastUsedAt},
				ClientSecretTruncated: secret.DisplaySecret,
			})
		}
		httpapi.Write(ctx, rw, http.StatusOK, secrets)
	}
}

// CreateAppSecret returns an http.HandlerFunc that handles POST /oauth2-provider/apps/{app}/secrets
func CreateAppSecret(db database.Store, auditor *audit.Auditor, logger slog.Logger) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		var (
			ctx               = r.Context()
			app               = httpmw.OAuth2ProviderApp(r)
			aReq, commitAudit = audit.InitRequest[database.OAuth2ProviderAppSecret](rw, &audit.RequestParams{
				Audit:   *auditor,
				Log:     logger,
				Request: r,
				Action:  database.AuditActionCreate,
			})
		)
		defer commitAudit()
		secret, err := GenerateSecret()
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to generate OAuth2 client secret.",
				Detail:  err.Error(),
			})
			return
		}
		dbSecret, err := db.InsertOAuth2ProviderAppSecret(ctx, database.InsertOAuth2ProviderAppSecretParams{
			ID:           uuid.New(),
			CreatedAt:    dbtime.Now(),
			SecretPrefix: []byte(secret.Prefix),
			HashedSecret: secret.Hashed,
			// DisplaySecret is the last six characters of the original unhashed secret.
			// This is done so they can be differentiated and it matches how GitHub
			// displays their client secrets.
			DisplaySecret: secret.Formatted[len(secret.Formatted)-6:],
			AppID:         app.ID,
		})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error creating OAuth2 client secret.",
				Detail:  err.Error(),
			})
			return
		}
		aReq.New = dbSecret
		httpapi.Write(ctx, rw, http.StatusCreated, codersdk.OAuth2ProviderAppSecretFull{
			ID:               dbSecret.ID,
			ClientSecretFull: secret.Formatted,
		})
	}
}

// DeleteAppSecret returns an http.HandlerFunc that handles DELETE /oauth2-provider/apps/{app}/secrets/{secretID}
func DeleteAppSecret(db database.Store, auditor *audit.Auditor, logger slog.Logger) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		var (
			ctx               = r.Context()
			secret            = httpmw.OAuth2ProviderAppSecret(r)
			aReq, commitAudit = audit.InitRequest[database.OAuth2ProviderAppSecret](rw, &audit.RequestParams{
				Audit:   *auditor,
				Log:     logger,
				Request: r,
				Action:  database.AuditActionDelete,
			})
		)
		aReq.Old = secret
		defer commitAudit()
		err := db.DeleteOAuth2ProviderAppSecretByID(ctx, secret.ID)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error deleting OAuth2 client secret.",
				Detail:  err.Error(),
			})
			return
		}
		rw.WriteHeader(http.StatusNoContent)
	}
}
