package oauth2provider_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/audit"
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

// TestCreateDynamicClientRegistration is a focused unit test on the RFC 7591
// handler itself, bypassing the full coderdtest HTTP server. It covers
// D1-01/D1-02: whether a client_secret is minted, and what client_type is
// persisted, must follow the requested token_endpoint_auth_method.
func TestCreateDynamicClientRegistration(t *testing.T) {
	t.Parallel()

	accessURL, err := url.Parse("https://oauth2-registration-test.example.com")
	require.NoError(t, err)

	tests := []struct {
		name string
		req  codersdk.OAuth2ClientRegistrationRequest

		wantStatus     int
		wantClientType string // Only checked when wantStatus is 201.
		wantSecret     bool   // Only checked when wantStatus is 201.
	}{
		{
			name: "DefaultAuthMethodIsConfidential",
			req: codersdk.OAuth2ClientRegistrationRequest{
				RedirectURIs: []string{"https://example.com/callback"},
			},
			wantStatus:     http.StatusCreated,
			wantClientType: "confidential",
			wantSecret:     true,
		},
		{
			name: "ClientSecretPostIsConfidential",
			req: codersdk.OAuth2ClientRegistrationRequest{
				RedirectURIs:            []string{"https://example.com/callback"},
				TokenEndpointAuthMethod: codersdk.OAuth2TokenEndpointAuthMethodClientSecretPost,
			},
			wantStatus:     http.StatusCreated,
			wantClientType: "confidential",
			wantSecret:     true,
		},
		{
			name: "NoneIsPublicWithNoSecret",
			req: codersdk.OAuth2ClientRegistrationRequest{
				RedirectURIs:            []string{"https://example.com/callback"},
				TokenEndpointAuthMethod: codersdk.OAuth2TokenEndpointAuthMethodNone,
			},
			wantStatus:     http.StatusCreated,
			wantClientType: "public",
			wantSecret:     false,
		},
		{
			name:       "MissingRedirectURIsIsRejected",
			req:        codersdk.OAuth2ClientRegistrationRequest{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			db, _ := dbtestutil.NewDB(t)
			err := db.UpsertOAuth2DCREnabled(ctx, true)
			require.NoError(t, err)

			logger := slogtest.Make(t, nil)
			auditor := audit.NewNop()
			// audit.InitRequest requires the ResponseWriter to be a
			// *tracing.StatusWriter, which normally comes from the
			// middleware chain in coderd.go; wrap it here to match.
			handler := tracing.StatusWriterMiddleware(oauth2provider.CreateDynamicClientRegistration(db, accessURL, &auditor, logger))

			body, err := json.Marshal(tt.req)
			require.NoError(t, err)

			r := httptest.NewRequest(http.MethodPost, "/oauth2/register", bytes.NewReader(body)).WithContext(ctx)
			r.Header.Set("Content-Type", "application/json")
			rw := httptest.NewRecorder()

			handler.ServeHTTP(rw, r)
			require.Equal(t, tt.wantStatus, rw.Code)
			if tt.wantStatus != http.StatusCreated {
				return
			}

			var resp codersdk.OAuth2ClientRegistrationResponse
			require.NoError(t, json.Unmarshal(rw.Body.Bytes(), &resp))

			if tt.wantSecret {
				require.NotEmpty(t, resp.ClientSecret)
			} else {
				require.Empty(t, resp.ClientSecret)
			}

			clientID, err := uuid.Parse(resp.ClientID)
			require.NoError(t, err)

			app, err := db.GetOAuth2ProviderAppByClientID(ctx, clientID)
			require.NoError(t, err)
			require.Equal(t, tt.wantClientType, app.ClientType.String)

			secrets, err := db.GetOAuth2ProviderAppSecretsByAppID(ctx, clientID)
			require.NoError(t, err)
			if tt.wantSecret {
				require.Len(t, secrets, 1)
			} else {
				require.Empty(t, secrets)
			}
		})
	}
}
