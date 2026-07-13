package oauth2provider_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
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
// handler itself, bypassing the full coderdtest HTTP server. It verifies that
// whether a client_secret is minted, and what client_type is persisted,
// follows the requested token_endpoint_auth_method.
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

// TestCreateDynamicClientRegistration_Transaction verifies that the app
// insert and the secret insert share a single database transaction, so a
// failure partway through can't leave a permanently committed, orphaned app
// row with no matching secret.
//
// A mock store is used because the failure needs to be injected between the
// two inserts, which isn't reachable through the public registration API
// against a real database (the failing insert's unique secret_prefix is
// generated internally and can't be forced to collide from the outside).
// mDB.EXPECT().InTx(...).Times(1) is the key assertion: it fails the test if
// the two inserts are ever changed back to being independent, unwrapped
// db.InsertX calls, since that path never calls InTx at all.
func TestCreateDynamicClientRegistration_Transaction(t *testing.T) {
	t.Parallel()

	accessURL, err := url.Parse("https://oauth2-registration-tx-test.example.com")
	require.NoError(t, err)

	tests := []struct {
		name            string
		secretInsertErr error
		wantStatus      int
	}{
		{
			name:       "BothInsertsShareOneTransaction",
			wantStatus: http.StatusCreated,
		},
		{
			name:            "SecretInsertFailureFailsTheWholeRegistration",
			secretInsertErr: xerrors.New("simulated secret insert failure"),
			wantStatus:      http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			ctrl := gomock.NewController(t)
			mDB := dbmock.NewMockStore(ctrl)

			// InTx must invoke the closure against the same store handle
			// (the mock itself, standing in for `tx`) so the two inserts
			// below are recorded as happening inside one shared
			// transaction, not as two independently committed statements.
			mDB.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
				func(f func(database.Store) error, _ *database.TxOptions) error {
					return f(mDB)
				},
			).Times(1)

			appCall := mDB.EXPECT().InsertOAuth2ProviderApp(gomock.Any(), gomock.Any()).
				Return(database.OAuth2ProviderApp{
					ID:         uuid.New(),
					ClientType: sql.NullString{String: "confidential", Valid: true},
				}, nil).
				Times(1)

			secretCall := mDB.EXPECT().InsertOAuth2ProviderAppSecret(gomock.Any(), gomock.Any()).
				Return(database.OAuth2ProviderAppSecret{}, tt.secretInsertErr).
				Times(1)

			// The secret insert can only run after the app insert, matching
			// registration.go's literal ordering inside the InTx closure.
			gomock.InOrder(appCall, secretCall)

			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: tt.secretInsertErr != nil})
			auditor := audit.NewNop()
			handler := tracing.StatusWriterMiddleware(oauth2provider.CreateDynamicClientRegistration(mDB, accessURL, &auditor, logger))

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
		})
	}
}
