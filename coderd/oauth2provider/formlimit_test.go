package oauth2provider_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/oauth2provider"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestFormEndpointsBodyTooLarge covers the form-parsing OAuth2 endpoints when
// client_id arrives in the query string.
//
// ExtractOAuth2ProviderAppWithOAuth2Errors installs the body bound, but it
// parses the form only when client_id is absent from the query. When client_id
// is present the middleware resolves the app without touching the body, so the
// handler's own r.ParseForm is the first read and the bound trips there. These
// endpoints authenticate the client from the body and therefore carry no API key
// middleware, so this read is still pre-authentication.
func TestFormEndpointsBodyTooLarge(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		path    string
		handler func(database.Store) http.Handler
	}{
		{
			name: "Tokens",
			path: "/oauth2/tokens",
			handler: func(db database.Store) http.Handler {
				return oauth2provider.Tokens(db, codersdk.SessionLifetime{})
			},
		},
		{
			name: "Revoke",
			path: "/oauth2/revoke",
			handler: func(db database.Store) http.Handler {
				return oauth2provider.RevokeToken(db, slogtest.Make(t, nil))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			db, _ := dbtestutil.NewDB(t)
			app := dbgen.OAuth2ProviderApp(t, db, database.OAuth2ProviderApp{})

			handler := httpmw.ExtractOAuth2ProviderAppWithOAuth2Errors(db)(tc.handler(db))

			// Padded past the limit so size is the only reason to reject. The
			// form itself is well formed, so a rejection for any other reason
			// would carry a different error code.
			body := "grant_type=authorization_code&token=abc&pad=" +
				strings.Repeat("a", httpapi.DefaultMaxRequestBodyBytes)
			require.Greater(t, len(body), httpapi.DefaultMaxRequestBodyBytes)

			r := httptest.NewRequest(http.MethodPost, tc.path+"?client_id="+app.ID.String(),
				strings.NewReader(body)).WithContext(ctx)
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rw := httptest.NewRecorder()

			handler.ServeHTTP(rw, r)

			require.Equal(t, http.StatusRequestEntityTooLarge, rw.Code,
				"response body: %s", rw.Body.String())

			var errResp codersdk.OAuth2Error
			require.NoError(t, json.Unmarshal(rw.Body.Bytes(), &errResp))
			require.Equal(t, codersdk.OAuth2ErrorCodeInvalidRequest, errResp.Error)
			require.Contains(t, errResp.ErrorDescription, strconv.Itoa(httpapi.DefaultMaxRequestBodyBytes))
		})
	}
}
