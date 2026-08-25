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
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
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

func TestCreateDynamicClientRegistration_ClientType(t *testing.T) {
	t.Parallel()

	accessURL, err := url.Parse("https://oauth2-registration-client-type-test.example.com")
	require.NoError(t, err)

	tests := []struct {
		name string
		req  codersdk.OAuth2ClientRegistrationRequest

		wantClientType string
		wantSecret     bool
	}{
		{
			name: "DefaultAuthMethodIsConfidential",
			req: codersdk.OAuth2ClientRegistrationRequest{
				RedirectURIs: []string{"https://example.com/callback"},
			},
			wantClientType: "confidential",
			wantSecret:     true,
		},
		{
			name: "ClientSecretPostIsConfidential",
			req: codersdk.OAuth2ClientRegistrationRequest{
				RedirectURIs:            []string{"https://example.com/callback"},
				TokenEndpointAuthMethod: codersdk.OAuth2TokenEndpointAuthMethodClientSecretPost,
			},
			wantClientType: "confidential",
			wantSecret:     true,
		},
		{
			name: "NoneIsPublicWithNoSecret",
			req: codersdk.OAuth2ClientRegistrationRequest{
				RedirectURIs:            []string{"https://example.com/callback"},
				TokenEndpointAuthMethod: codersdk.OAuth2TokenEndpointAuthMethodNone,
			},
			wantClientType: "public",
			wantSecret:     false,
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
			handler := tracing.StatusWriterMiddleware(oauth2provider.CreateDynamicClientRegistration(db, accessURL, &auditor, logger))

			body, err := json.Marshal(tt.req)
			require.NoError(t, err)

			r := httptest.NewRequest(http.MethodPost, "/oauth2/register", bytes.NewReader(body)).WithContext(ctx)
			r.Header.Set("Content-Type", "application/json")
			rw := httptest.NewRecorder()

			handler.ServeHTTP(rw, r)
			require.Equal(t, http.StatusCreated, rw.Code)

			var resp codersdk.OAuth2ClientRegistrationResponse
			require.NoError(t, json.Unmarshal(rw.Body.Bytes(), &resp))

			// RFC 7591 §3.2.1: client_secret is omitted entirely for a client
			// that was not issued one. Assert against the raw body, because a
			// decoded struct cannot tell an absent key from a present empty
			// one, and key presence is exactly what a client branches on.
			var rawBody map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(rw.Body.Bytes(), &rawBody))
			if tt.wantSecret {
				require.Contains(t, rawBody, "client_secret")
				require.NotEmpty(t, resp.ClientSecret)
			} else {
				require.NotContains(t, rawBody, "client_secret")
			}

			clientID, err := uuid.Parse(resp.ClientID)
			require.NoError(t, err)

			app, err := db.GetOAuth2ProviderAppByClientID(ctx, clientID)
			require.NoError(t, err)
			require.Equal(t, tt.wantClientType, app.ClientType)

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

// TestCreateDynamicClientRegistration_RegistrationClientURI pins the
// accessURL.JoinPath fix: a trailing slash on the configured access URL
// used to mint "//oauth2/clients/{id}" via fmt.Sprintf. Without this test,
// reverting to fmt.Sprintf would pass every other test in this file, since
// none of them configures a trailing-slash accessURL.
func TestCreateDynamicClientRegistration_RegistrationClientURI(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	accessURL, err := url.Parse("https://oauth2-registration-client-uri-test.example.com/")
	require.NoError(t, err)

	db, _ := dbtestutil.NewDB(t)
	require.NoError(t, db.UpsertOAuth2DCREnabled(ctx, true))

	logger := slogtest.Make(t, nil)
	auditor := audit.NewNop()
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
	require.Equal(t, http.StatusCreated, rw.Code)

	var resp codersdk.OAuth2ClientRegistrationResponse
	require.NoError(t, json.Unmarshal(rw.Body.Bytes(), &resp))

	wantURI := "https://oauth2-registration-client-uri-test.example.com/oauth2/clients/" + resp.ClientID
	require.Equal(t, wantURI, resp.RegistrationClientURI)
}

// TestCreateDynamicClientRegistration_Transaction verifies that the app insert
// and the secret insert share a single database transaction, so a failure
// partway through can't leave a permanently committed, orphaned app row with
// no matching secret.
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
			// A separate handle for the transaction, so a call made on the
			// outer store is distinguishable from one made on tx.
			mTx := dbmock.NewMockStore(ctrl)

			mDB.EXPECT().GetOAuth2DCREnabled(gomock.Any()).Return(true, nil).Times(1)

			mDB.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
				func(f func(database.Store) error, _ *database.TxOptions) error {
					return f(mTx)
				},
			).Times(1)

			// Both expectations live on mTx, not mDB, and that is the
			// assertion. An insert issued on the outer store, whether after
			// InTx returns or from inside the closure against the wrong
			// handle, lands on mDB, which has no expectation for it, so
			// gomock fails the unexpected call. Registering both on a single
			// mock makes inside and outside the transaction indistinguishable
			// and the test then passes either way, which is the trap the
			// project's own InTx rule exists to catch
			// (.claude/docs/DATABASE.md).
			// Echoes params.ClientType rather than hardcoding it, so the mock
			// stays honest about what the handler asked for; a hardcoded
			// value would keep passing if a later change reused this
			// scaffolding for a public request without updating it.
			appCall := mTx.EXPECT().InsertOAuth2ProviderApp(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, params database.InsertOAuth2ProviderAppParams) (database.OAuth2ProviderApp, error) {
					return database.OAuth2ProviderApp{
						ID:         uuid.New(),
						ClientType: params.ClientType,
					}, nil
				}).
				Times(1)

			secretCall := mTx.EXPECT().InsertOAuth2ProviderAppSecret(gomock.Any(), gomock.Any()).
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

// TestCreateDynamicClientRegistration_PublicClientSkipsSecretInsert verifies
// that a public client's registration issues no InsertOAuth2ProviderAppSecret
// call at all, rather than inserting a row with an empty secret. This pins
// the mechanism (no call made); TestCreateDynamicClientRegistration_ClientType
// /NoneIsPublicWithNoSecret pins the outcome (no row exists) against a real
// database. Both are kept: a handler that inserts an empty-secret row would
// pass the outcome test's require.Empty only if the read side also filters
// it out, so the mechanism test is the one that fails at the actual call
// site if that regresses.
func TestCreateDynamicClientRegistration_PublicClientSkipsSecretInsert(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	accessURL, err := url.Parse("https://oauth2-registration-public-tx-test.example.com")
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	mDB := dbmock.NewMockStore(ctrl)
	mTx := dbmock.NewMockStore(ctrl)

	mDB.EXPECT().GetOAuth2DCREnabled(gomock.Any()).Return(true, nil).Times(1)
	mDB.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
		func(f func(database.Store) error, _ *database.TxOptions) error {
			return f(mTx)
		},
	).Times(1)
	mTx.EXPECT().InsertOAuth2ProviderApp(gomock.Any(), gomock.Any()).
		Return(database.OAuth2ProviderApp{
			ID:         uuid.New(),
			ClientType: database.OAuth2ProviderAppClientTypePublic,
		}, nil).
		Times(1)
	// The absence of an InsertOAuth2ProviderAppSecret expectation on either
	// handle is the assertion: gomock fails on an unexpected call. This only
	// holds because the two handles are distinct, so a secret insert issued
	// anywhere is unexpected rather than absorbed by a shared expectation.

	logger := slogtest.Make(t, nil)
	auditor := audit.NewNop()
	handler := tracing.StatusWriterMiddleware(oauth2provider.CreateDynamicClientRegistration(mDB, accessURL, &auditor, logger))

	req := codersdk.OAuth2ClientRegistrationRequest{
		RedirectURIs:            []string{"https://example.com/callback"},
		TokenEndpointAuthMethod: codersdk.OAuth2TokenEndpointAuthMethodNone,
	}
	body, err := json.Marshal(req)
	require.NoError(t, err)

	r := httptest.NewRequest(http.MethodPost, "/oauth2/register", bytes.NewReader(body)).WithContext(ctx)
	r.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, r)
	require.Equal(t, http.StatusCreated, rw.Code)
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

func TestClientConfiguration_ReportedAuthMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		clientType string
		// A row predating the column reads back empty whether NULL or "".
		stored sql.NullString
		// Sent on PUT. Must match stored or agree with the stored client type,
		// or the type-change guard rejects the update first.
		resend codersdk.OAuth2TokenEndpointAuthMethod
		want   codersdk.OAuth2TokenEndpointAuthMethod
	}{
		{
			// A client echoing back the raw stored method, not the reported one.
			name:       "ConfidentialStoringNoneResendingStored",
			clientType: database.OAuth2ProviderAppClientTypeConfidential,
			stored:     sql.NullString{String: string(codersdk.OAuth2TokenEndpointAuthMethodNone), Valid: true},
			resend:     codersdk.OAuth2TokenEndpointAuthMethodNone,
			want:       codersdk.OAuth2TokenEndpointAuthMethodClientSecretBasic,
		},
		{
			// A GET-driven client, whose write leaves the two columns agreeing.
			name:       "ConfidentialStoringNoneResendingReported",
			clientType: database.OAuth2ProviderAppClientTypeConfidential,
			stored:     sql.NullString{String: string(codersdk.OAuth2TokenEndpointAuthMethodNone), Valid: true},
			resend:     codersdk.OAuth2TokenEndpointAuthMethodClientSecretBasic,
			want:       codersdk.OAuth2TokenEndpointAuthMethodClientSecretBasic,
		},
		{
			name:       "PublicStoringSecretMethodResendingStored",
			clientType: database.OAuth2ProviderAppClientTypePublic,
			stored:     sql.NullString{String: string(codersdk.OAuth2TokenEndpointAuthMethodClientSecretBasic), Valid: true},
			resend:     codersdk.OAuth2TokenEndpointAuthMethodClientSecretBasic,
			want:       codersdk.OAuth2TokenEndpointAuthMethodNone,
		},
		{
			name:       "PublicStoringSecretMethodResendingReported",
			clientType: database.OAuth2ProviderAppClientTypePublic,
			stored:     sql.NullString{String: string(codersdk.OAuth2TokenEndpointAuthMethodClientSecretBasic), Valid: true},
			resend:     codersdk.OAuth2TokenEndpointAuthMethodNone,
			want:       codersdk.OAuth2TokenEndpointAuthMethodNone,
		},
		{
			// RFC 7591 §2 defaults an unspecified method to client_secret_basic,
			// which is also what ApplyDefaults substitutes on update.
			name:       "ConfidentialStoringNothing",
			clientType: database.OAuth2ProviderAppClientTypeConfidential,
			stored:     sql.NullString{String: "", Valid: true},
			resend:     codersdk.OAuth2TokenEndpointAuthMethodClientSecretBasic,
			want:       codersdk.OAuth2TokenEndpointAuthMethodClientSecretBasic,
		},
		{
			// The only agreeing pair whose reported method differs from the type
			// default, separating stored-method reporting from a type switch.
			name:       "ConfidentialStoringClientSecretPost",
			clientType: database.OAuth2ProviderAppClientTypeConfidential,
			stored:     sql.NullString{String: string(codersdk.OAuth2TokenEndpointAuthMethodClientSecretPost), Valid: true},
			resend:     codersdk.OAuth2TokenEndpointAuthMethodClientSecretPost,
			want:       codersdk.OAuth2TokenEndpointAuthMethodClientSecretPost,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			db, _ := dbtestutil.NewDB(t)

			// Seeded directly: registration now derives client_type from the
			// method, so it can no longer produce a disagreeing row.
			app := dbgen.OAuth2ProviderApp(t, db, database.OAuth2ProviderApp{
				CallbackURL:             "https://example.com/callback",
				RedirectUris:            []string{"https://example.com/callback"},
				ClientType:              tt.clientType,
				TokenEndpointAuthMethod: tt.stored,
				DynamicallyRegistered:   sql.NullBool{Bool: true, Valid: true},
			})

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("client_id", app.ID.String())
			getReq := httptest.NewRequest(http.MethodGet, "/oauth2/clients/"+app.ID.String(), nil).
				WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
			getRW := httptest.NewRecorder()

			oauth2provider.GetClientConfiguration(db).ServeHTTP(getRW, getReq)
			require.Equal(t, http.StatusOK, getRW.Code, "body: %s", getRW.Body.String())

			var got codersdk.OAuth2ClientConfiguration
			require.NoError(t, json.NewDecoder(getRW.Body).Decode(&got))
			require.Equal(t, tt.want, got.TokenEndpointAuthMethod)

			logger := slogtest.Make(t, nil)
			auditor := audit.NewNop()
			handler := tracing.StatusWriterMiddleware(oauth2provider.UpdateClientConfiguration(db, &auditor, logger))

			body, err := json.Marshal(codersdk.OAuth2ClientRegistrationRequest{
				RedirectURIs:            []string{"https://example.com/callback"},
				TokenEndpointAuthMethod: tt.resend,
			})
			require.NoError(t, err)

			putRCtx := chi.NewRouteContext()
			putRCtx.URLParams.Add("client_id", app.ID.String())
			putReq := httptest.NewRequest(http.MethodPut, "/oauth2/clients/"+app.ID.String(),
				bytes.NewReader(body)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, putRCtx))
			putReq.Header.Set("Content-Type", "application/json")
			putRW := httptest.NewRecorder()

			handler.ServeHTTP(putRW, putReq)
			require.Equal(t, http.StatusOK, putRW.Code, "body: %s", putRW.Body.String())

			var updated codersdk.OAuth2ClientConfiguration
			require.NoError(t, json.NewDecoder(putRW.Body).Decode(&updated))
			require.Equal(t, tt.want, updated.TokenEndpointAuthMethod)

			// The row keeps what the client sent, so the response can still
			// disagree with it. That is what lets the divergence heal.
			stored, err := db.GetOAuth2ProviderAppByClientID(ctx, app.ID)
			require.NoError(t, err)
			require.Equal(t, string(tt.resend), stored.TokenEndpointAuthMethod.String)
			require.Equal(t, tt.clientType, stored.ClientType)
		})
	}
}
