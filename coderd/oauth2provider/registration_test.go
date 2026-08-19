package oauth2provider_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/oauth2provider"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestCreateDynamicClientRegistration_DCREnabled is a focused unit test on
// the RFC 7591 handler itself, bypassing the full coderdtest HTTP server. It
// verifies the dynamic-client-registration-enabled gate: registration
// succeeds once an admin explicitly enables DCR, is rejected with 403 when
// explicitly disabled, and defaults to disabled when the setting has never
// been configured.
func TestCreateDynamicClientRegistration_DCREnabled(t *testing.T) {
	t.Parallel()

	accessURL, err := url.Parse("https://oauth2-registration-dcr-test.example.com")
	require.NoError(t, err)

	tests := []struct {
		name string
		// configureDCR is nil for "never configured".
		configureDCR *bool
		wantStatus   int
	}{
		{
			name:         "EnabledAllowsRegistration",
			configureDCR: ptr.Ref(true),
			wantStatus:   http.StatusCreated,
		},
		{
			name:         "DisabledRejectsRegistration",
			configureDCR: ptr.Ref(false),
			wantStatus:   http.StatusForbidden,
		},
		{
			name:         "NeverConfiguredDefaultsToDisabled",
			configureDCR: nil,
			wantStatus:   http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			db, _ := dbtestutil.NewDB(t)
			if tt.configureDCR != nil {
				err := db.UpsertOAuth2DCREnabled(ctx, *tt.configureDCR)
				require.NoError(t, err)
			}

			logger := slogtest.Make(t, nil)
			auditor := audit.NewNop()
			// audit.InitRequest requires the ResponseWriter to be a
			// *tracing.StatusWriter, which normally comes from the
			// middleware chain in coderd.go; wrap it here to match.
			handler := tracing.StatusWriterMiddleware(oauth2provider.CreateDynamicClientRegistration(db, accessURL, &auditor, logger))

			req := codersdk.OAuth2ClientRegistrationRequest{
				RedirectURIs: []string{"https://example.com/callback"},
			}
			body, err := json.Marshal(req)
			require.NoError(t, err)

			r := httptest.NewRequest(http.MethodPost, "/oauth2/register", bytes.NewReader(body)).WithContext(ctx)
			r.Header.Set("Content-Type", "application/json")
			rw := httptest.NewRecorder()

			handler.ServeHTTP(rw, r)
			require.Equal(t, tt.wantStatus, rw.Code)

			if tt.wantStatus != http.StatusForbidden {
				return
			}

			var errResp map[string]string
			require.NoError(t, json.Unmarshal(rw.Body.Bytes(), &errResp))
			require.Equal(t, "invalid_request", errResp["error"])
			require.Contains(t, errResp["error_description"], "disabled")
		})
	}
}

// TestUpdateClientConfiguration_ClientTypeIsImmutable verifies that an
// RFC 7592 update cannot move a registered client between public and
// confidential. Allowing it would either drop the secret requirement for a
// client that has a secret, or mark a client confidential when it has no
// secret and no way to be issued one, permanently breaking its token
// exchange. Switching between the two confidential auth methods stays
// allowed, since it changes nothing about how the client authenticates.
func TestUpdateClientConfiguration_ClientTypeIsImmutable(t *testing.T) {
	t.Parallel()

	accessURL, err := url.Parse("https://oauth2-registration-immutable-type-test.example.com")
	require.NoError(t, err)

	tests := []struct {
		name       string
		registerAs codersdk.OAuth2TokenEndpointAuthMethod
		updateTo   codersdk.OAuth2TokenEndpointAuthMethod
		// omitAuthMethod sends the update with no token_endpoint_auth_method at
		// all, which ApplyDefaults rewrites to client_secret_basic before the
		// guard sees it. updateTo is ignored when set.
		omitAuthMethod    bool
		wantStatus        int
		wantFinalCallback string
		// wantClientType is deliberately a bare literal rather than the
		// database constant: it pins the value actually stored in the column,
		// so it must fail if that spelling ever changes. Fixtures that set
		// state use the constant instead.
		wantClientType string
	}{
		{
			name:           "ConfidentialToPublicIsRejected",
			registerAs:     codersdk.OAuth2TokenEndpointAuthMethodClientSecretBasic,
			updateTo:       codersdk.OAuth2TokenEndpointAuthMethodNone,
			wantStatus:     http.StatusBadRequest,
			wantClientType: "confidential",
		},
		{
			name:           "PublicToConfidentialIsRejected",
			registerAs:     codersdk.OAuth2TokenEndpointAuthMethodNone,
			updateTo:       codersdk.OAuth2TokenEndpointAuthMethodClientSecretBasic,
			wantStatus:     http.StatusBadRequest,
			wantClientType: "public",
		},
		{
			// RFC 7592 makes PUT a full replacement, so an omitted auth method
			// defaults to client_secret_basic and moves a public client to
			// confidential, which is rejected. The rejection is correct; what
			// matters is that it is reported in terms the caller can act on,
			// since they never sent the field named in the error.
			name:           "PublicWithOmittedAuthMethodIsRejected",
			registerAs:     codersdk.OAuth2TokenEndpointAuthMethodNone,
			omitAuthMethod: true,
			wantStatus:     http.StatusBadRequest,
			wantClientType: "public",
		},
		{
			// Both are confidential, so the guard must not fire.
			name:              "BasicToPostIsAllowed",
			registerAs:        codersdk.OAuth2TokenEndpointAuthMethodClientSecretBasic,
			updateTo:          codersdk.OAuth2TokenEndpointAuthMethodClientSecretPost,
			wantStatus:        http.StatusOK,
			wantFinalCallback: "https://example.com/updated-callback",
			wantClientType:    "confidential",
		},
		{
			// A confidential client omitting the field is unaffected, because
			// the default it lands on is also confidential. Pinned so the
			// asymmetry with the public case above stays visible.
			name:              "ConfidentialWithOmittedAuthMethodIsAllowed",
			registerAs:        codersdk.OAuth2TokenEndpointAuthMethodClientSecretPost,
			omitAuthMethod:    true,
			wantStatus:        http.StatusOK,
			wantFinalCallback: "https://example.com/updated-callback",
			wantClientType:    "confidential",
		},
		{
			name:              "PublicToPublicIsAllowed",
			registerAs:        codersdk.OAuth2TokenEndpointAuthMethodNone,
			updateTo:          codersdk.OAuth2TokenEndpointAuthMethodNone,
			wantStatus:        http.StatusOK,
			wantFinalCallback: "https://example.com/updated-callback",
			wantClientType:    "public",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			db, _ := dbtestutil.NewDB(t)
			require.NoError(t, db.UpsertOAuth2DCREnabled(ctx, true))

			logger := slogtest.Make(t, nil)
			auditor := audit.NewNop()

			// Register the client first, so the update runs against a real
			// persisted client_type rather than a hand-built fixture.
			createHandler := tracing.StatusWriterMiddleware(oauth2provider.CreateDynamicClientRegistration(db, accessURL, &auditor, logger))
			createBody, err := json.Marshal(codersdk.OAuth2ClientRegistrationRequest{
				RedirectURIs:            []string{"https://example.com/callback"},
				TokenEndpointAuthMethod: tt.registerAs,
			})
			require.NoError(t, err)

			createReq := httptest.NewRequest(http.MethodPost, "/oauth2/register", bytes.NewReader(createBody)).WithContext(ctx)
			createReq.Header.Set("Content-Type", "application/json")
			createRW := httptest.NewRecorder()
			createHandler.ServeHTTP(createRW, createReq)
			require.Equal(t, http.StatusCreated, createRW.Code)

			var created codersdk.OAuth2ClientRegistrationResponse
			require.NoError(t, json.Unmarshal(createRW.Body.Bytes(), &created))
			clientID, err := uuid.Parse(created.ClientID)
			require.NoError(t, err)

			updateHandler := tracing.StatusWriterMiddleware(oauth2provider.UpdateClientConfiguration(db, &auditor, logger))
			updateReqBody := codersdk.OAuth2ClientRegistrationRequest{
				RedirectURIs: []string{"https://example.com/updated-callback"},
			}
			if !tt.omitAuthMethod {
				updateReqBody.TokenEndpointAuthMethod = tt.updateTo
			}
			updateBody, err := json.Marshal(updateReqBody)
			require.NoError(t, err)

			// The handler reads client_id via chi.URLParam, which normally
			// comes from the router in coderd.go.
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("client_id", clientID.String())
			updateCtx := context.WithValue(ctx, chi.RouteCtxKey, rctx)

			updateReq := httptest.NewRequest(http.MethodPut, "/oauth2/clients/"+clientID.String(), bytes.NewReader(updateBody)).WithContext(updateCtx)
			updateReq.Header.Set("Content-Type", "application/json")
			updateRW := httptest.NewRecorder()
			updateHandler.ServeHTTP(updateRW, updateReq)
			require.Equal(t, tt.wantStatus, updateRW.Code)

			app, err := db.GetOAuth2ProviderAppByClientID(ctx, clientID)
			require.NoError(t, err)

			// client_type is what IsPublic() reads to decide whether the token
			// endpoint validates a secret, so it must be unchanged whether the
			// update was accepted or rejected.
			require.Equal(t, tt.wantClientType, app.ClientType)

			if tt.wantStatus != http.StatusOK {
				var errResp map[string]string
				require.NoError(t, json.Unmarshal(updateRW.Body.Bytes(), &errResp))
				require.Equal(t, "invalid_client_metadata", errResp["error"])
				// The error code alone cannot distinguish this guard from
				// req.Validate() failing, which returns the same one, and
				// neither can the untouched row below. The description is the
				// only field that tells them apart, so assert on it: a change
				// that made "none" fail validation outright would otherwise
				// leave these cases green while testing something else.
				require.Contains(t, errResp["error_description"], "cannot move an existing client between public and confidential")
				// It must also name what the server actually compared, since
				// the caller may never have sent the field.
				require.Contains(t, errResp["error_description"], "client_secret_basic")
				// The rejection must leave the whole update unapplied, not
				// just the client_type field.
				require.Equal(t, "https://example.com/callback", app.CallbackURL)
				return
			}

			require.Equal(t, tt.wantFinalCallback, app.CallbackURL)
			wantMethod := tt.updateTo
			if tt.omitAuthMethod {
				// ApplyDefaults substitutes the RFC 7591 default.
				wantMethod = codersdk.OAuth2TokenEndpointAuthMethodClientSecretBasic
			}
			require.Equal(t, string(wantMethod), app.TokenEndpointAuthMethod.String)
		})
	}
}

// TestUpdateClientConfiguration_LegacyAuthMethodMismatch covers clients that
// registered before client_type was derived from token_endpoint_auth_method.
// Registration persisted whatever auth method was requested while hardcoding
// client_type to "confidential", and "none" has always passed validation, so
// apps stored as confidential with an auth method of "none" exist in any
// deployment where a native or MCP client self-registered. That is the exact
// population public clients are for.
//
// Such a client must still be able to manage its registration. Comparing only
// the derived client type would reject it forever, including when it resends
// the metadata GET reports, leaving re-registration as the only recovery. It
// must also not be silently converted to public, since it holds a secret that
// would stop being required.
func TestUpdateClientConfiguration_LegacyAuthMethodMismatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		updateTo codersdk.OAuth2TokenEndpointAuthMethod
	}{
		{
			// The read-modify-write shape: echo back what GET reports.
			name:     "ResendingStoredAuthMethodIsAccepted",
			updateTo: codersdk.OAuth2TokenEndpointAuthMethodNone,
		},
		{
			// Moving to a secret-based method matches the stored confidential
			// type, so it is allowed and repairs the divergence.
			name:     "MovingToSecretBasedMethodIsAccepted",
			updateTo: codersdk.OAuth2TokenEndpointAuthMethodClientSecretBasic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			db, _ := dbtestutil.NewDB(t)
			require.NoError(t, db.UpsertOAuth2DCREnabled(ctx, true))

			legacy := dbgen.OAuth2ProviderApp(t, db, database.OAuth2ProviderApp{
				CallbackURL:             "https://example.com/callback",
				RedirectUris:            []string{"https://example.com/callback"},
				ClientType:              database.OAuth2ProviderAppClientTypeConfidential,
				TokenEndpointAuthMethod: sql.NullString{String: "none", Valid: true},
				DynamicallyRegistered:   sql.NullBool{Bool: true, Valid: true},
			})
			// Registration issued a secret unconditionally back then.
			_ = dbgen.OAuth2ProviderAppSecret(t, db, database.OAuth2ProviderAppSecret{AppID: legacy.ID})

			logger := slogtest.Make(t, nil)
			auditor := audit.NewNop()
			handler := tracing.StatusWriterMiddleware(oauth2provider.UpdateClientConfiguration(db, &auditor, logger))

			body, err := json.Marshal(codersdk.OAuth2ClientRegistrationRequest{
				RedirectURIs:            []string{"https://example.com/updated-callback"},
				TokenEndpointAuthMethod: tt.updateTo,
			})
			require.NoError(t, err)

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("client_id", legacy.ID.String())
			r := httptest.NewRequest(http.MethodPut, "/oauth2/clients/"+legacy.ID.String(),
				bytes.NewReader(body)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
			r.Header.Set("Content-Type", "application/json")
			rw := httptest.NewRecorder()

			handler.ServeHTTP(rw, r)
			require.Equal(t, http.StatusOK, rw.Code, "body: %s", rw.Body.String())

			app, err := db.GetOAuth2ProviderAppByClientID(ctx, legacy.ID)
			require.NoError(t, err)
			require.Equal(t, "https://example.com/updated-callback", app.CallbackURL)
			require.Equal(t, string(tt.updateTo), app.TokenEndpointAuthMethod.String)

			// The update must not convert the client to public. It still holds
			// a secret, and IsPublic() reading "public" here would stop the
			// token endpoint from requiring it.
			require.Equal(t, "confidential", app.ClientType)
			require.False(t, app.IsPublic())
			secrets, err := db.GetOAuth2ProviderAppSecretsByAppID(ctx, legacy.ID)
			require.NoError(t, err)
			require.Len(t, secrets, 1)
		})
	}
}
