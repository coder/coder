package oauthpki_test

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
	"github.com/coder/coder/v2/coderd/oauthpki"
	"github.com/coder/coder/v2/testutil"
)

//nolint:gosec // these are just for testing
const (
	testClientKey = `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEAnUryZEfn5kA8wuk9a7ogFuWbk3uPHEhioYuAg9m3/tIdqSqu
ASpRzw8+1nORTf3ykWRRlhxZWnKimmkB0Ux5Yrz9TDVWDQbzEH3B8ibMlmaNcoN8
wYVzeEpqCe3fJagnV0lh0sHB1Z+vhcJ/M2nEAdyfhIgQEbG6Xtl2+WcGqyMWUJpV
g8+ebK+JkXELAGN1hg3DdV52gjodEjoe1/ibHz8y3NR7j2tOKix7iKOhccyFkD35
xqSnfyZJK5yxIfmGiWdVOIGqc2rYpgvrXJLTOjLoeyDSNi+Q604T64ZxsqfuM4LX
BakVG3EwHFXPBfsBKjUE9HYvXEXw3fJP9K6mIwIDAQABAoIBAQCb+aH7x0IylSir
r1Z06RDBI9bunOwBA9aqkwdRuCg4zGsVQXljNnABgACz7837JQPRIUW2MU553otX
yyE+RzNnsjkLxSgbqvSFOe+FDOx7iB5jm/euf4NNmZ0lU3iggurgJ6iVsgVgrQUF
AyXX+d2gawLUDYjBwxgozkSodH2sXYSX+SWfSOXHsFzSa3tLtUMbAIflM0rlRXf7
Z57M8mMomZUvmmojH+TnBQljJlU8lhrvOaDD4DT8qAtVHE3VluDBQ9/3E8OIjz+E
EqUgWLgrdq1rIMhJbHN90NwLwWs+2PcRfdB6hqKPktLne2KZFOgVKlxPKOYByBq1
PX/vJ/HBAoGBAMFmJ6nYqyUVl26ajlXmnXBjQ+iBLHo9lcUu84+rpqRf90Bsm5bd
jMmYr3Yo3yXNiit3rvZzBfPElo+IVa1HpPtgOaa2AU5B3QzxWCNT0FNRQqMG2LcA
CvB10pOdJEABQxr7d4eFRg2/KbF1fr0r0vqMEelwa5ejTg6ROD3DtadpAoGBANA0
4EClniCwvd1IECy2oTuTDosXgmRKwRAcwgE34YXy1Y/L4X/ghFeCHi3ybrep0uwL
ptJNK+0sqvPu6UhC356GfMqfuzOKNMkXybnPUbHrz5KTkN+QQMfPc73Veel2gpD3
xNataEmHtxcOx0X1OnjwyZZpmMbrUY3Cackn+durAoGBAKYR5nU+jJfnloVvSlIR
GZhsZN++LEc7ouQTkSoJp6r2jQZRPLmrvT1PUzwPlK6NdNwmhaMy2iWc5fySgZ+u
KcmBs3+oQi7E9+ApThnn2rfwy1vagTWDX+FkC1KeWYZsjwcYcGd61dDwGgk8b3xZ
qW1j4e2mj31CycBQiw7eg5ohAoGADvkOe3etlHpBXS12hFCp7afYruYE6YN6uNbo
mL/VBxX8h7fIwrJ5sfVYiENb9PdQhMsdtxf3pbnFnX875Ydxn2vag5PTGZTB0QhV
6HfhTyM/LTJRg9JS5kuj7i3w83ojT5uR20JjMo6A+zaD3CMTjmj6hkeXxg5cMg6e
HuoyDLsCgYBcbboYMFT1cUSxBeMtPGt3CxxZUYnUQaRUeOcjqYYlFL+DCWhY7pxH
EnLhwW/KzkDzOmwRmmNOMqD7UhR/ayxR+avRt6v5d5l8fVCuNexgs7kR9L5IQp9l
YV2wsCoXBCcuPmio/te44U//BlzprEu0w1iHpb3ibmQg4y291R0TvQ==
-----END RSA PRIVATE KEY-----`

	testClientCert = `
-----BEGIN CERTIFICATE-----
MIIEOjCCAiKgAwIBAgIQMO50KnWsRbmrrthPQgyubjANBgkqhkiG9w0BAQsFADAY
MRYwFAYDVQQDEw1Mb2NhbGhvc3RDZXJ0MB4XDTIzMDgxMDE2MjYxOFoXDTI1MDIx
MDE2MjU0M1owFDESMBAGA1UEAxMJbG9jYWxob3N0MIIBIjANBgkqhkiG9w0BAQEF
AAOCAQ8AMIIBCgKCAQEAnUryZEfn5kA8wuk9a7ogFuWbk3uPHEhioYuAg9m3/tId
qSquASpRzw8+1nORTf3ykWRRlhxZWnKimmkB0Ux5Yrz9TDVWDQbzEH3B8ibMlmaN
coN8wYVzeEpqCe3fJagnV0lh0sHB1Z+vhcJ/M2nEAdyfhIgQEbG6Xtl2+WcGqyMW
UJpVg8+ebK+JkXELAGN1hg3DdV52gjodEjoe1/ibHz8y3NR7j2tOKix7iKOhccyF
kD35xqSnfyZJK5yxIfmGiWdVOIGqc2rYpgvrXJLTOjLoeyDSNi+Q604T64Zxsqfu
M4LXBakVG3EwHFXPBfsBKjUE9HYvXEXw3fJP9K6mIwIDAQABo4GDMIGAMA4GA1Ud
DwEB/wQEAwIDuDAdBgNVHSUEFjAUBggrBgEFBQcDAQYIKwYBBQUHAwIwHQYDVR0O
BBYEFAYCdgydG3h2SNWF+BfAyJtNliJtMB8GA1UdIwQYMBaAFHR/aptP0RUNNFyf
5uky527SECt1MA8GA1UdEQQIMAaHBH8AAAEwDQYJKoZIhvcNAQELBQADggIBAI6P
ymG7l06JvJ3p6xgaMyOxgkpQl6WkY4LJHVEhfeDSoO3qsJc4PxUdSExJsT84weXb
lF+tK6D/CPlvjmG720IlB5cSKJ71rWjwmaMWKxWKXyoZdDrHAp55+FNdXegUZF2o
EF/ZM5CHaO8iHMkuWEv1OASHBQWC/o4spUN5HGQ9HepwLVvO/aX++LYfvfL9faKA
IT+w9i8pJbfItFmfA8x2OEVZk8aEA0WtKdfsMwzGmZ1GSGa4UYcynxQGCMiB5h4L
C/dpoJRbEzdGLuTZgV2SCaN3k5BrH4aaILI9tqZaq0gamN9Rd2yji3cGiduCeAAo
RmVcl9fBliMLxylWEP5+B2JmCZEc8Lfm0TBNnjaG17KY40gzbfBYixBxBTYgsPua
bfprtfksSG++zcsDbkC8CtPamtlNWtDAiFp4yQRkP79PlJO6qCdTrFWPukTMCMso
25hjLvxj1fLy/jSMDEZu/oQ14TMCZSGHRjz4CPiaCfXqgqOtVOD+5+yWInwUGp/i
Nb1vIq4ruEAbyCbdWKHbE0yT5AP7hm5ZNybpZ4/311AEBD2HKip/OqB05p99XcLw
BIC4ODNvwCn6x00KZoqWz/MX2dEQ/HqWiWaDB/OSemfTVE3I94mzEWnqpF2cQpcT
B1B7CpkMU55hPP+7nsofCszNrMDXT8Z5w2a3zLKM
-----END CERTIFICATE-----
`
)

// TestAzureADPKIOIDC ensures we do not break Azure AD compatibility.
// It runs an oauth2.Exchange method and hijacks the request to only check the
// request side of the transaction.
func TestAzureADPKIOIDC(t *testing.T) {
	t.Parallel()

	oauthCfg := &oauth2.Config{
		ClientID: "random-client-id",
		Endpoint: oauth2.Endpoint{
			TokenURL: "https://login.microsoftonline.com/6a1e9139-13f2-4afb-8f46-036feac8bd79/v2.0/token",
		},
	}

	pkiConfig, err := oauthpki.NewOauth2PKIConfig(oauthpki.ConfigParams{
		ClientID:       oauthCfg.ClientID,
		TokenURL:       oauthCfg.Endpoint.TokenURL,
		PemEncodedKey:  []byte(testClientKey),
		PemEncodedCert: []byte(testClientCert),
		Config:         oauthCfg,
		Scopes:         []string{"openid", "email", "profile"},
	})
	require.NoError(t, err, "failed to create pki config")

	ctx := testutil.Context(t, testutil.WaitMedium)
	ctx = oidc.ClientContext(ctx, &http.Client{
		Transport: &fakeRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				resp := &http.Response{
					Status: "500 Internal Service Error",
				}
				// This is the easiest way to hijack the request and check
				// the params. The oauth2 package uses unexported types and
				// options, so we need to view the actual request created.
				assertJWTAuth(t, req)
				return resp, nil
			},
		},
	})
	_, err = pkiConfig.Exchange(ctx, base64.StdEncoding.EncodeToString([]byte("random-code")))
	// We hijack the request and return an error intentionally
	require.Error(t, err, "error expected")
}

// TestAzureAKPKIWithCoderd uses a fake IDP and a real Coderd to test PKI auth.
// nolint:bodyclose
func TestAzureAKPKIWithCoderd(t *testing.T) {
	t.Parallel()

	scopes := []string{"openid", "email", "profile", "offline_access"}
	fake := oidctest.NewFakeIDP(t,
		oidctest.WithIssuer("https://login.microsoftonline.com/fake_app"),
		oidctest.WithCustomClientAuth(func(t testing.TB, req *http.Request) (url.Values, error) {
			values := assertJWTAuth(t, req)
			if values == nil {
				return nil, xerrors.New("authorizatin failed in request")
			}
			return values, nil
		}),
		oidctest.WithServing(),
	)
	cfg := fake.OIDCConfig(t, scopes, func(cfg *coderd.OIDCConfig) {
		cfg.AllowSignups = true
	})

	oauthCfg := cfg.OAuth2Config.(*oauth2.Config)
	// Create the oauthpki config
	pki, err := oauthpki.NewOauth2PKIConfig(oauthpki.ConfigParams{
		ClientID:       oauthCfg.ClientID,
		TokenURL:       oauthCfg.Endpoint.TokenURL,
		Scopes:         scopes,
		PemEncodedKey:  []byte(testClientKey),
		PemEncodedCert: []byte(testClientCert),
		Config:         oauthCfg,
	})
	require.NoError(t, err)
	cfg.OAuth2Config = pki

	owner, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		OIDCConfig: cfg,
	})

	// Create a user and login
	const email = "alice@coder.com"
	claims := jwt.MapClaims{
		"email": email,
		"sub":   uuid.NewString(),
	}
	helper := oidctest.NewLoginHelper(owner, fake)
	user, _ := helper.Login(t, claims)

	// Try refreshing the token more than once.
	for i := 0; i < 2; i++ {
		helper.ForceRefresh(t, api.Database, user, claims)
	}
}

// TestSavedAzureADPKIOIDC was created by capturing actual responses from an Azure
// AD instance and saving them to replay, removing some details.
// The reason this is done is that this is the only way to assert values
// passed to the oauth2 provider via http requests.
// It is not feasible to run against an actual Azure AD instance, so this attempts
// to prevent some regressions by running a full "e2e" oauth and asserting some
// of the request values.
func TestSavedAzureADPKIOIDC(t *testing.T) {
	t.Parallel()

	var (
		stateString = "random-state"
		oauth2Code  = base64.StdEncoding.EncodeToString([]byte("random-code"))
	)

	// Real oauth config. We will hijack all http requests so some of these values
	// are fake.
	cfg := &oauth2.Config{
		ClientID:     "fake_app",
		ClientSecret: "",
		Endpoint: oauth2.Endpoint{
			AuthURL:   "https://login.microsoftonline.com/fake_app/oauth2/v2.0/authorize",
			TokenURL:  "https://login.microsoftonline.com/fake_app/oauth2/v2.0/token",
			AuthStyle: 0,
		},
		RedirectURL: "http://localhost/api/v2/users/oidc/callback",
		Scopes:      []string{"openid", "profile", "email", "offline_access"},
	}

	initialExchange := false
	tokenRefreshed := false

	// Create the oauthpki config
	pki, err := oauthpki.NewOauth2PKIConfig(oauthpki.ConfigParams{
		ClientID:       cfg.ClientID,
		TokenURL:       cfg.Endpoint.TokenURL,
		Scopes:         []string{"openid", "email", "profile", "offline_access"},
		PemEncodedKey:  []byte(testClientKey),
		PemEncodedCert: []byte(testClientCert),
		Config:         cfg,
	})
	require.NoError(t, err)

	var fakeCtx context.Context
	fakeClient := &http.Client{
		Transport: fakeRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.String(), "authorize") {
					// This is the user hitting the browser endpoint to begin the OIDC
					// auth flow.

					// Authorize should redirect the user back to the app after authentication on
					// the IDP.
					resp := httptest.NewRecorder()
					v := url.Values{
						"code":          {oauth2Code},
						"state":         {stateString},
						"session_state": {"a18cf797-1e2b-4bc3-baf9-66b41a4997cf"},
					}

					// This url doesn't really matter since the fake client will hiject this actual request.
					http.Redirect(resp, req, "http://localhost:3000/api/v2/users/oidc/callback?"+v.Encode(), http.StatusTemporaryRedirect)
					return resp.Result(), nil
				}
				if strings.Contains(req.URL.String(), "v2.0/token") {
					vals := assertJWTAuth(t, req)
					switch vals.Get("grant_type") {
					case "authorization_code":
						// Initial token
						initialExchange = true
						assert.Equal(t, oauth2Code, vals.Get("code"), "initial exchange code mismatch")
					case "refresh_token":
						// refreshed token
						tokenRefreshed = true
						assert.Equal(t, "<refresh_token_JWT>", vals.Get("refresh_token"), "refresh token required")
					}

					resp := httptest.NewRecorder()
					// Taken from an actual response
					// Just always return a token no matter what.
					resp.Header().Set("Content-Type", "application/json")
					_, _ = resp.Write([]byte(`{
					   "token_type":"Bearer",
					   "scope":"email openid profile AccessReview.ReadWrite.Membership Group.Read.All Group.ReadWrite.All User.Read",
					   "expires_in":4009,
					   "ext_expires_in":4009,
					   "access_token":"<access_token_JWT>",
					   "refresh_token":"<refresh_token_JWT>",
					   "id_token":"eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiIsImtpZCI6Ii1LSTNROW5OUjdiUm9meG1lWm9YcWJIWkdldyJ9.eyJhdWQiOiIxZjAxODMyYS1mZWViLTQyZGMtODFkOS01ZjBhYjZhMDQxZTAiLCJpc3MiOiJodHRwczovL2xvZ2luLm1pY3Jvc29mdG9ubGluZS5jb20vMTEwZjBjMGYtY2Q3Ni00NzE3LWE2ZjgtNGVlYTNkMGY4MTA5L3YyLjAiLCJpYXQiOjE2OTE3OTI2MzQsIm5iZiI6MTY5MTc5MjYzNCwiZXhwIjoxNjkxNzk2NTM0LCJhaW8iOiJBWVFBZS84VUFBQUE1eEtqMmVTdWFXVmZsRlhCeGJJTnMvSkVyVHFvUGlaQW5ENmJIZWF3a2RRcisyRVRwM3RGNGY3akxicnh3ODhhVm9QOThrY0xMNjhON1hVV3FCN1I1N2JQRU9EclRlSUI1S0lyUHBjbCtIeXR0a1ljOVdWQklVVEErSllQbzl1a0ZjbGNWZ1krWUc3eHlmdi90K3Q1ZEczblNuZEdEQ1FYRVIxbDlTNko1T2c9IiwiZW1haWwiOiJzdGV2ZW5AY29kZXIuY29tIiwiZ3JvdXBzIjpbImM4MDQ4ZTkxLWY1YzMtNDdlNS05NjkzLTgzNGRlODQwMzRhZCIsIjcwYjQ4MTc1LTEwN2ItNGFkOC1iNDA1LTRkODg4YTFjNDY2ZiJdLCJpZHAiOiJtYWlsIiwibmFtZSI6IlN0ZXZlbiBNIiwib2lkIjoiN2JhNDYzNjAtZTAyNy00OTVhLTlhZTUtM2FlYWZlMzY3MGEyIiwicHJlZmVycmVkX3VzZXJuYW1lIjoic3RldmVuQGNvZGVyLmNvbSIsInByb3ZfZGF0YSI6W3siQXQiOnRydWUsIlByb3YiOiJnaXRodWIuY29tIiwiQWx0c2VjaWQiOiI1NDQ2Mjk4IiwiQWNjZXNzVG9rZW4iOm51bGx9XSwicmgiOiIwLkFUZ0FEd3dQRVhiTkYwZW0tRTdxUFEtQkNTcURBUl9yX3R4Q2dkbGZDcmFnUWVBNEFPRS4iLCJyb2xlcyI6WyJUZW1wbGF0ZUF1dGhvcnMiXSwic3ViIjoib0JTN3FjUERKdWlDMEYyQ19XdDJycVlvanhpT0o3S3JFWjlkQ1RkTGVYNCIsInRpZCI6IjExMGYwYzBmLWNkNzYtNDcxNy1hNmY4LTRlZWEzZDBmODEwOSIsInV0aSI6IktReGlIWGtaZUVxcC1tQWlVdTlyQUEiLCJ2ZXIiOiIyLjAiLCJyb2xlczIiOiJUZW1wbGF0ZUF1dGhvcnMifQ.JevFI4Xm9dW7kQq4xEgZnUaU0SqbeOAFtT0YIKQNefR9Db4sjxCaKRmX0pPt-CM9j45d6fAiAkLFDAqjlSbi4Zi0GbEomT3yegmuxKgEgjPpJlGjF2TBUpsNNyn5gJ9Wkct9BfwALJhX2ePJFzIlkvx9opNNbNK1qHKMMjOSRFG6AGExKRDiQAME0a4hVgCwrAdUs4JrCcj4LqB84dODN-eoh-jx2-1wDvf6fovfwLHDQwjY4lfBxaYdNavKM369hrhU-U067rSnCzvDD26f4VLhPF52hiQIbTVN5t7p_1XmcduUiaNnmr9AZiZxZ-94mctSRRR8xG0pNwO2yv84iA"
					}`))
					return resp.Result(), nil
				}
				// This is the "Coder" half of things. We can keep this in the fake
				// client, essentially being the fake client on both sides of the OIDC
				// flow.
				if strings.Contains(req.URL.String(), "v2/users/oidc/callback") {
					// This is the callback from the IDP.
					code := req.URL.Query().Get("code")
					require.Equal(t, oauth2Code, code, "code mismatch")
					state := req.URL.Query().Get("state")
					require.Equal(t, stateString, state, "state mismatch")

					// Exchange for token should work
					token, err := pki.Exchange(fakeCtx, code)
					if !assert.NoError(t, err) {
						return httptest.NewRecorder().Result(), nil
					}

					// Also try a refresh
					cpy := token
					cpy.Expiry = time.Now().Add(time.Minute * -1)
					src := pki.TokenSource(fakeCtx, cpy)
					_, err = src.Token()
					tokenRefreshed = true
					assert.NoError(t, err, "token refreshed")
					return httptest.NewRecorder().Result(), nil
				}

				return nil, xerrors.Errorf("not implemented")
			},
		},
	}
	fakeCtx = oidc.ClientContext(context.Background(), fakeClient)
	_ = fakeCtx

	// This simulates a client logging into the browser. The 307 redirect will
	// make sure this goes through the full flow.
	// nolint: noctx
	resp, err := fakeClient.Get(pki.AuthCodeURL("state", oauth2.AccessTypeOffline))
	require.NoError(t, err)
	_ = resp.Body.Close()

	require.True(t, initialExchange, "initial token exchange complete")
	require.True(t, tokenRefreshed, "token was refreshed")
}

type fakeRoundTripper struct {
	roundTrip func(req *http.Request) (*http.Response, error)
}

func (f fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f.roundTrip(req)
}

// assertJWTAuth will assert the basic JWT auth assertions. It will return the
// url.Values from the request body for any additional assertions to be made.
func assertJWTAuth(t testing.TB, r *http.Request) url.Values {
	body, err := io.ReadAll(r.Body)
	if !assert.NoError(t, err) {
		return nil
	}
	vals, err := url.ParseQuery(string(body))
	if !assert.NoError(t, err) {
		return nil
	}

	assert.Equal(t, "urn:ietf:params:oauth:client-assertion-type:jwt-bearer", vals.Get("client_assertion_type"))
	jwtToken := vals.Get("client_assertion")
	// No need to actually verify the jwt is signed right.
	parsedToken, _, err := (&jwt.Parser{}).ParseUnverified(jwtToken, jwt.MapClaims{})
	if !assert.NoError(t, err, "failed to parse jwt token") {
		return nil
	}

	// Azure requirements
	assert.NotEmpty(t, parsedToken.Header["x5t"], "hashed cert missing")
	assert.Equal(t, "RS256", parsedToken.Header["alg"], "azure only accepts RS256")
	assert.Equal(t, "JWT", parsedToken.Header["typ"], "azure only accepts JWT")

	return vals
}
