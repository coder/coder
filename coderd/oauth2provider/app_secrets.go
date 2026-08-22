package oauth2provider

import (
	"net/http"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
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

		// A public client authenticates with PKCE alone, so the token endpoint
		// never validates a secret for one. Minting a secret anyway would hand
		// an operator a credential that does nothing, and worse, one whose
		// deletion looks like a kill switch: for a confidential app deleting a
		// secret cascades its tokens away, but a public client's tokens carry a
		// NULL app_secret_id, so deleting it revokes nothing.
		if app.IsPublic() {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Cannot create a client secret for a public OAuth2 app.",
				Detail:  "Public clients authenticate with PKCE and have no client secret. The client type is fixed at registration.",
			})
			return
		}

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
