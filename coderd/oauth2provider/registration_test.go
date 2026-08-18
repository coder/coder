package oauth2provider_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/httpapi"
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

// TestCreateDynamicClientRegistration_BodyTooLarge asserts that an oversized
// body is rejected with an RFC 7591 error body rather than a codersdk.Response.
// Every other error in this handler is protocol-shaped, and a client that parses
// the OAuth2 error shape would fail to read a codersdk.Response, so the status
// alone is not sufficient.
func TestCreateDynamicClientRegistration_BodyTooLarge(t *testing.T) {
	t.Parallel()

	accessURL, err := url.Parse("https://oauth2-registration-too-large-test.example.com")
	require.NoError(t, err)

	ctx := testutil.Context(t, testutil.WaitLong)

	db, _ := dbtestutil.NewDB(t)
	require.NoError(t, db.UpsertOAuth2DCREnabled(ctx, true))

	logger := slogtest.Make(t, nil)
	auditor := audit.NewNop()
	handler := tracing.StatusWriterMiddleware(oauth2provider.CreateDynamicClientRegistration(db, accessURL, &auditor, logger))

	// One valid redirect URI padded past the limit, so size is the only reason
	// to reject the request.
	req := codersdk.OAuth2ClientRegistrationRequest{
		RedirectURIs: []string{"https://example.com/callback"},
		ClientName:   strings.Repeat("a", httpapi.DefaultMaxRequestBodyBytes),
	}
	body, err := json.Marshal(req)
	require.NoError(t, err)
	require.Greater(t, len(body), httpapi.DefaultMaxRequestBodyBytes)

	r := httptest.NewRequest(http.MethodPost, "/oauth2/register", bytes.NewReader(body)).WithContext(ctx)
	r.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, r)
	require.Equal(t, http.StatusRequestEntityTooLarge, rw.Code)

	var errResp map[string]string
	require.NoError(t, json.Unmarshal(rw.Body.Bytes(), &errResp))
	require.Equal(t, "invalid_request", errResp["error"])
	require.Contains(t, errResp["error_description"], strconv.Itoa(httpapi.DefaultMaxRequestBodyBytes))
}
