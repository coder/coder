package oauth2provider

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// CreateDynamicClientRegistration returns an http.HandlerFunc that handles POST /oauth2/register
func CreateDynamicClientRegistration(db database.Store, accessURL *url.URL, auditor *audit.Auditor, logger slog.Logger) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		aReq, commitAudit := audit.InitRequest[database.OAuth2ProviderApp](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
		defer commitAudit()

		// Parse request
		var req codersdk.OAuth2ClientRegistrationRequest
		if !httpapi.Read(ctx, rw, r, &req) {
			return
		}

		// Validate request
		if err := req.Validate(); err != nil {
			writeOAuth2RegistrationError(ctx, rw, http.StatusBadRequest,
				"invalid_client_metadata", err.Error())
			return
		}

		// Apply defaults
		req = req.ApplyDefaults()

		// Generate client credentials
		clientID := uuid.New()
		clientSecret, hashedSecret, err := generateClientCredentials()
		if err != nil {
			writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
				"server_error", "Failed to generate client credentials")
			return
		}

		// Generate registration access token for RFC 7592 management
		registrationToken, hashedRegToken, err := generateRegistrationAccessToken()
		if err != nil {
			writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
				"server_error", "Failed to generate registration token")
			return
		}

		// Store in database - use system context since this is a public endpoint
		now := dbtime.Now()
		clientName := req.GenerateClientName()
		//nolint:gocritic // Dynamic client registration is a public endpoint, system access required
		app, err := db.InsertOAuth2ProviderApp(dbauthz.AsSystemRestricted(ctx), database.InsertOAuth2ProviderAppParams{
			ID:                      clientID,
			CreatedAt:               now,
			UpdatedAt:               now,
			Name:                    clientName,
			Icon:                    req.LogoURI,
			CallbackURL:             req.RedirectURIs[0], // Primary redirect URI
			RedirectUris:            req.RedirectURIs,
			ClientType:              sql.NullString{String: req.DetermineClientType(), Valid: true},
			DynamicallyRegistered:   sql.NullBool{Bool: true, Valid: true},
			ClientIDIssuedAt:        sql.NullTime{Time: now, Valid: true},
			ClientSecretExpiresAt:   sql.NullTime{}, // No expiration for now
			GrantTypes:              enumSliceToStrings(req.GrantTypes),
			ResponseTypes:           enumSliceToStrings(req.ResponseTypes),
			TokenEndpointAuthMethod: sql.NullString{String: string(req.TokenEndpointAuthMethod), Valid: true},
			Scope:                   sql.NullString{String: req.Scope, Valid: true},
			Contacts:                req.Contacts,
			ClientUri:               sql.NullString{String: req.ClientURI, Valid: req.ClientURI != ""},
			LogoUri:                 sql.NullString{String: req.LogoURI, Valid: req.LogoURI != ""},
			TosUri:                  sql.NullString{String: req.TOSURI, Valid: req.TOSURI != ""},
			PolicyUri:               sql.NullString{String: req.PolicyURI, Valid: req.PolicyURI != ""},
			JwksUri:                 sql.NullString{String: req.JWKSURI, Valid: req.JWKSURI != ""},
			Jwks:                    pqtype.NullRawMessage{RawMessage: req.JWKS, Valid: len(req.JWKS) > 0},
			SoftwareID:              sql.NullString{String: req.SoftwareID, Valid: req.SoftwareID != ""},
			SoftwareVersion:         sql.NullString{String: req.SoftwareVersion, Valid: req.SoftwareVersion != ""},
			RegistrationAccessToken: hashedRegToken,
			RegistrationClientUri:   sql.NullString{String: fmt.Sprintf("%s/oauth2/clients/%s", accessURL.String(), clientID), Valid: true},
		})
		if err != nil {
			logger.Error(ctx, "failed to store oauth2 client registration",
				slog.Error(err),
				slog.F("client_name", clientName),
				slog.F("client_id", clientID.String()),
				slog.F("redirect_uris", req.RedirectURIs))
			writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
				"server_error", "Failed to store client registration")
			return
		}

		// Create client secret - parse the formatted secret to get components
		parsedSecret, err := ParseFormattedSecret(clientSecret)
		if err != nil {
			writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
				"server_error", "Failed to parse generated secret")
			return
		}

		//nolint:gocritic // Dynamic client registration is a public endpoint, system access required
		_, err = db.InsertOAuth2ProviderAppSecret(dbauthz.AsSystemRestricted(ctx), database.InsertOAuth2ProviderAppSecretParams{
			ID:            uuid.New(),
			CreatedAt:     now,
			SecretPrefix:  []byte(parsedSecret.Prefix),
			HashedSecret:  hashedSecret,
			DisplaySecret: createDisplaySecret(clientSecret),
			AppID:         clientID,
		})
		if err != nil {
			writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
				"server_error", "Failed to store client secret")
			return
		}

		// Set audit log data
		aReq.New = app

		// Return response
		response := codersdk.OAuth2ClientRegistrationResponse{
			ClientID:                app.ID.String(),
			ClientSecret:            clientSecret,
			ClientIDIssuedAt:        app.ClientIDIssuedAt.Time.Unix(),
			ClientSecretExpiresAt:   0, // No expiration
			RedirectURIs:            app.RedirectUris,
			ClientName:              app.Name,
			ClientURI:               app.ClientUri.String,
			LogoURI:                 app.LogoUri.String,
			TOSURI:                  app.TosUri.String,
			PolicyURI:               app.PolicyUri.String,
			JWKSURI:                 app.JwksUri.String,
			JWKS:                    app.Jwks.RawMessage,
			SoftwareID:              app.SoftwareID.String,
			SoftwareVersion:         app.SoftwareVersion.String,
			GrantTypes:              stringsToEnumSlice[codersdk.OAuth2ProviderGrantType](app.GrantTypes),
			ResponseTypes:           stringsToEnumSlice[codersdk.OAuth2ProviderResponseType](app.ResponseTypes),
			TokenEndpointAuthMethod: codersdk.OAuth2TokenEndpointAuthMethod(app.TokenEndpointAuthMethod.String),
			Scope:                   app.Scope.String,
			Contacts:                app.Contacts,
			RegistrationAccessToken: registrationToken,
			RegistrationClientURI:   app.RegistrationClientUri.String,
		}

		httpapi.Write(ctx, rw, http.StatusCreated, response)
	}
}

// GetClientConfiguration returns an http.HandlerFunc that handles GET /oauth2/clients/{client_id}
func GetClientConfiguration(db database.Store) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Extract client ID from URL path
		clientIDStr := chi.URLParam(r, "client_id")
		clientID, err := uuid.Parse(clientIDStr)
		if err != nil {
			writeOAuth2RegistrationError(ctx, rw, http.StatusBadRequest,
				"invalid_client_metadata", "Invalid client ID format")
			return
		}

		// Get app by client ID
		//nolint:gocritic // RFC 7592 endpoints need system access to retrieve dynamically registered clients
		app, err := db.GetOAuth2ProviderAppByClientID(dbauthz.AsSystemRestricted(ctx), clientID)
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				writeOAuth2RegistrationError(ctx, rw, http.StatusUnauthorized,
					"invalid_token", "Client not found")
			} else {
				writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
					"server_error", "Failed to retrieve client")
			}
			return
		}

		// Check if client was dynamically registered
		if !app.DynamicallyRegistered.Bool {
			writeOAuth2RegistrationError(ctx, rw, http.StatusUnauthorized,
				"invalid_token", "Client was not dynamically registered")
			return
		}

		// Return client configuration (without client_secret for security)
		response := codersdk.OAuth2ClientConfiguration{
			ClientID:                app.ID.String(),
			ClientIDIssuedAt:        app.ClientIDIssuedAt.Time.Unix(),
			ClientSecretExpiresAt:   0, // No expiration for now
			RedirectURIs:            app.RedirectUris,
			ClientName:              app.Name,
			ClientURI:               app.ClientUri.String,
			LogoURI:                 app.LogoUri.String,
			TOSURI:                  app.TosUri.String,
			PolicyURI:               app.PolicyUri.String,
			JWKSURI:                 app.JwksUri.String,
			JWKS:                    app.Jwks.RawMessage,
			SoftwareID:              app.SoftwareID.String,
			SoftwareVersion:         app.SoftwareVersion.String,
			GrantTypes:              stringsToEnumSlice[codersdk.OAuth2ProviderGrantType](app.GrantTypes),
			ResponseTypes:           stringsToEnumSlice[codersdk.OAuth2ProviderResponseType](app.ResponseTypes),
			TokenEndpointAuthMethod: codersdk.OAuth2TokenEndpointAuthMethod(app.TokenEndpointAuthMethod.String),
			Scope:                   app.Scope.String,
			Contacts:                app.Contacts,
			RegistrationAccessToken: "", // RFC 7592: Not returned in GET responses for security
			RegistrationClientURI:   app.RegistrationClientUri.String,
		}

		httpapi.Write(ctx, rw, http.StatusOK, response)
	}
}

// UpdateClientConfiguration returns an http.HandlerFunc that handles PUT /oauth2/clients/{client_id}
func UpdateClientConfiguration(db database.Store, auditor *audit.Auditor, logger slog.Logger) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		aReq, commitAudit := audit.InitRequest[database.OAuth2ProviderApp](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
		defer commitAudit()

		// Extract client ID from URL path
		clientIDStr := chi.URLParam(r, "client_id")
		clientID, err := uuid.Parse(clientIDStr)
		if err != nil {
			writeOAuth2RegistrationError(ctx, rw, http.StatusBadRequest,
				"invalid_client_metadata", "Invalid client ID format")
			return
		}

		// Parse request
		var req codersdk.OAuth2ClientRegistrationRequest
		if !httpapi.Read(ctx, rw, r, &req) {
			return
		}

		// Validate request
		if err := req.Validate(); err != nil {
			writeOAuth2RegistrationError(ctx, rw, http.StatusBadRequest,
				"invalid_client_metadata", err.Error())
			return
		}

		// Apply defaults
		req = req.ApplyDefaults()

		// Get existing app to verify it exists and is dynamically registered
		//nolint:gocritic // RFC 7592 endpoints need system access to retrieve dynamically registered clients
		existingApp, err := db.GetOAuth2ProviderAppByClientID(dbauthz.AsSystemRestricted(ctx), clientID)
		if err == nil {
			aReq.Old = existingApp
		}
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				writeOAuth2RegistrationError(ctx, rw, http.StatusUnauthorized,
					"invalid_token", "Client not found")
			} else {
				writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
					"server_error", "Failed to retrieve client")
			}
			return
		}

		// Check if client was dynamically registered
		if !existingApp.DynamicallyRegistered.Bool {
			writeOAuth2RegistrationError(ctx, rw, http.StatusForbidden,
				"invalid_token", "Client was not dynamically registered")
			return
		}

		// Update app in database
		now := dbtime.Now()
		//nolint:gocritic // RFC 7592 endpoints need system access to update dynamically registered clients
		updatedApp, err := db.UpdateOAuth2ProviderAppByClientID(dbauthz.AsSystemRestricted(ctx), database.UpdateOAuth2ProviderAppByClientIDParams{
			ID:                      clientID,
			UpdatedAt:               now,
			Name:                    req.GenerateClientName(),
			Icon:                    req.LogoURI,
			CallbackURL:             req.RedirectURIs[0], // Primary redirect URI
			RedirectUris:            req.RedirectURIs,
			ClientType:              sql.NullString{String: req.DetermineClientType(), Valid: true},
			ClientSecretExpiresAt:   sql.NullTime{}, // No expiration for now
			GrantTypes:              enumSliceToStrings(req.GrantTypes),
			ResponseTypes:           enumSliceToStrings(req.ResponseTypes),
			TokenEndpointAuthMethod: sql.NullString{String: string(req.TokenEndpointAuthMethod), Valid: true},
			Scope:                   sql.NullString{String: req.Scope, Valid: true},
			Contacts:                req.Contacts,
			ClientUri:               sql.NullString{String: req.ClientURI, Valid: req.ClientURI != ""},
			LogoUri:                 sql.NullString{String: req.LogoURI, Valid: req.LogoURI != ""},
			TosUri:                  sql.NullString{String: req.TOSURI, Valid: req.TOSURI != ""},
			PolicyUri:               sql.NullString{String: req.PolicyURI, Valid: req.PolicyURI != ""},
			JwksUri:                 sql.NullString{String: req.JWKSURI, Valid: req.JWKSURI != ""},
			Jwks:                    pqtype.NullRawMessage{RawMessage: req.JWKS, Valid: len(req.JWKS) > 0},
			SoftwareID:              sql.NullString{String: req.SoftwareID, Valid: req.SoftwareID != ""},
			SoftwareVersion:         sql.NullString{String: req.SoftwareVersion, Valid: req.SoftwareVersion != ""},
		})
		if err != nil {
			writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
				"server_error", "Failed to update client")
			return
		}

		// Set audit log data
		aReq.New = updatedApp

		// Return updated client configuration
		response := codersdk.OAuth2ClientConfiguration{
			ClientID:                updatedApp.ID.String(),
			ClientIDIssuedAt:        updatedApp.ClientIDIssuedAt.Time.Unix(),
			ClientSecretExpiresAt:   0, // No expiration for now
			RedirectURIs:            updatedApp.RedirectUris,
			ClientName:              updatedApp.Name,
			ClientURI:               updatedApp.ClientUri.String,
			LogoURI:                 updatedApp.LogoUri.String,
			TOSURI:                  updatedApp.TosUri.String,
			PolicyURI:               updatedApp.PolicyUri.String,
			JWKSURI:                 updatedApp.JwksUri.String,
			JWKS:                    updatedApp.Jwks.RawMessage,
			SoftwareID:              updatedApp.SoftwareID.String,
			SoftwareVersion:         updatedApp.SoftwareVersion.String,
			GrantTypes:              stringsToEnumSlice[codersdk.OAuth2ProviderGrantType](updatedApp.GrantTypes),
			ResponseTypes:           stringsToEnumSlice[codersdk.OAuth2ProviderResponseType](updatedApp.ResponseTypes),
			TokenEndpointAuthMethod: codersdk.OAuth2TokenEndpointAuthMethod(updatedApp.TokenEndpointAuthMethod.String),
			Scope:                   updatedApp.Scope.String,
			Contacts:                updatedApp.Contacts,
			RegistrationAccessToken: "", // RFC 7592: Not returned for security
			RegistrationClientURI:   updatedApp.RegistrationClientUri.String,
		}

		httpapi.Write(ctx, rw, http.StatusOK, response)
	}
}

// DeleteClientConfiguration returns an http.HandlerFunc that handles DELETE /oauth2/clients/{client_id}
func DeleteClientConfiguration(db database.Store, auditor *audit.Auditor, logger slog.Logger) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		aReq, commitAudit := audit.InitRequest[database.OAuth2ProviderApp](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     logger,
			Request: r,
			Action:  database.AuditActionDelete,
		})
		defer commitAudit()

		// Extract client ID from URL path
		clientIDStr := chi.URLParam(r, "client_id")
		clientID, err := uuid.Parse(clientIDStr)
		if err != nil {
			writeOAuth2RegistrationError(ctx, rw, http.StatusBadRequest,
				"invalid_client_metadata", "Invalid client ID format")
			return
		}

		// Get existing app to verify it exists and is dynamically registered
		//nolint:gocritic // RFC 7592 endpoints need system access to retrieve dynamically registered clients
		existingApp, err := db.GetOAuth2ProviderAppByClientID(dbauthz.AsSystemRestricted(ctx), clientID)
		if err == nil {
			aReq.Old = existingApp
		}
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				writeOAuth2RegistrationError(ctx, rw, http.StatusUnauthorized,
					"invalid_token", "Client not found")
			} else {
				writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
					"server_error", "Failed to retrieve client")
			}
			return
		}

		// Check if client was dynamically registered
		if !existingApp.DynamicallyRegistered.Bool {
			writeOAuth2RegistrationError(ctx, rw, http.StatusForbidden,
				"invalid_token", "Client was not dynamically registered")
			return
		}

		// Delete the client and all associated data (tokens, secrets, etc.)
		//nolint:gocritic // RFC 7592 endpoints need system access to delete dynamically registered clients
		err = db.DeleteOAuth2ProviderAppByClientID(dbauthz.AsSystemRestricted(ctx), clientID)
		if err != nil {
			writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
				"server_error", "Failed to delete client")
			return
		}

		// Note: audit data already set above with aReq.Old = existingApp

		// Return 204 No Content as per RFC 7592
		rw.WriteHeader(http.StatusNoContent)
	}
}

// RequireRegistrationAccessToken returns middleware that validates the registration access token for RFC 7592 endpoints
func RequireRegistrationAccessToken(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Extract client ID from URL path
			clientIDStr := chi.URLParam(r, "client_id")
			clientID, err := uuid.Parse(clientIDStr)
			if err != nil {
				writeOAuth2RegistrationError(ctx, rw, http.StatusBadRequest,
					"invalid_client_id", "Invalid client ID format")
				return
			}

			// Extract registration access token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeOAuth2RegistrationError(ctx, rw, http.StatusUnauthorized,
					"invalid_token", "Missing Authorization header")
				return
			}

			if !strings.HasPrefix(authHeader, "Bearer ") {
				writeOAuth2RegistrationError(ctx, rw, http.StatusUnauthorized,
					"invalid_token", "Authorization header must use Bearer scheme")
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == "" {
				writeOAuth2RegistrationError(ctx, rw, http.StatusUnauthorized,
					"invalid_token", "Missing registration access token")
				return
			}

			// Get the client and verify the registration access token
			//nolint:gocritic // RFC 7592 endpoints need system access to validate dynamically registered clients
			app, err := db.GetOAuth2ProviderAppByClientID(dbauthz.AsSystemRestricted(ctx), clientID)
			if err != nil {
				if xerrors.Is(err, sql.ErrNoRows) {
					// Return 401 for authentication-related issues, not 404
					writeOAuth2RegistrationError(ctx, rw, http.StatusUnauthorized,
						"invalid_token", "Client not found")
				} else {
					writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
						"server_error", "Failed to retrieve client")
				}
				return
			}

			// Check if client was dynamically registered
			if !app.DynamicallyRegistered.Bool {
				writeOAuth2RegistrationError(ctx, rw, http.StatusForbidden,
					"invalid_token", "Client was not dynamically registered")
				return
			}

			// Verify the registration access token
			if len(app.RegistrationAccessToken) == 0 {
				writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
					"server_error", "Client has no registration access token")
				return
			}

			// Compare the provided token with the stored hash
			if !apikey.ValidateHash(app.RegistrationAccessToken, token) {
				writeOAuth2RegistrationError(ctx, rw, http.StatusUnauthorized,
					"invalid_token", "Invalid registration access token")
				return
			}

			// Token is valid, continue to the next handler
			next.ServeHTTP(rw, r)
		})
	}
}

// Helper functions for RFC 7591 Dynamic Client Registration

// generateClientCredentials generates a client secret for OAuth2 apps
func generateClientCredentials() (plaintext string, hashed []byte, err error) {
	// Use the same pattern as existing OAuth2 app secrets
	secret, err := GenerateSecret()
	if err != nil {
		return "", nil, xerrors.Errorf("generate secret: %w", err)
	}

	return secret.Formatted, secret.Hashed, nil
}

// generateRegistrationAccessToken generates a registration access token for RFC 7592
func generateRegistrationAccessToken() (plaintext string, hashed []byte, err error) {
	return apikey.GenerateSecret(secretLength)
}

// writeOAuth2RegistrationError writes RFC 7591 compliant error responses
func writeOAuth2RegistrationError(_ context.Context, rw http.ResponseWriter, status int, errorCode, description string) {
	// RFC 7591 error response format
	errorResponse := map[string]string{
		"error": errorCode,
	}
	if description != "" {
		errorResponse["error_description"] = description
	}

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	_ = json.NewEncoder(rw).Encode(errorResponse)
}

// createDisplaySecret creates a display version of the secret showing only the last few characters
func createDisplaySecret(secret string) string {
	if len(secret) <= displaySecretLength {
		return secret
	}

	visiblePart := secret[len(secret)-displaySecretLength:]
	hiddenLength := len(secret) - displaySecretLength
	return strings.Repeat("*", hiddenLength) + visiblePart
}

func enumSliceToStrings[T ~string](enums []T) []string {
	result := make([]string, len(enums))
	for i, e := range enums {
		result[i] = string(e)
	}
	return result
}

func stringsToEnumSlice[T ~string](strs []string) []T {
	result := make([]T, len(strs))
	for i, s := range strs {
		result[i] = T(s)
	}
	return result
}
