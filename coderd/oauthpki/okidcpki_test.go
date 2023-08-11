package oauthpki_test

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/oauthpki"
	"github.com/coder/coder/testutil"
)

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

type exchangeAssert struct {
	httpmw.OAuth2Config
	assert func(ctx context.Context, code string, opts ...oauth2.AuthCodeOption)
}

func (a *exchangeAssert) Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	a.assert(ctx, code, opts...)
	return a.OAuth2Config.Exchange(ctx, code, opts...)
}

type fakeRoundTripper struct {
	roundTrip func(req *http.Request) (*http.Response, error)
}

func (f fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f.roundTrip(req)
}

// TestAzureADPKIOIDC ensures we do not break Azure AD compatibility.
func TestAzureADPKIOIDC(t *testing.T) {
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
				data, err := io.ReadAll(req.Body)
				if !assert.NoError(t, err, "failed to read request body") {
					return resp, nil
				}
				vals, err := url.ParseQuery(string(data))
				if !assert.NoError(t, err, "failed to parse values") {
					return resp, nil
				}
				assert.Equal(t, "urn:ietf:params:oauth:client-assertion-type:jwt-bearer", vals.Get("client_assertion_type"))

				jwtToken := vals.Get("client_assertion")

				// No need to actually verify the jwt is signed right.
				parsedToken, _, err := (&jwt.Parser{}).ParseUnverified(jwtToken, jwt.MapClaims{})
				if !assert.NoError(t, err, "failed to parse jwt token") {
					return resp, nil
				}

				// Azure requirements
				assert.NotEmpty(t, parsedToken.Header["x5t"], "hashed cert missing")
				assert.Equal(t, "RS256", parsedToken.Header["alg"], "azure only accepts RS256")
				assert.Equal(t, "JWT", parsedToken.Header["typ"], "azure only accepts JWT")
				return resp, nil
			},
		},
	})
	_, err = pkiConfig.Exchange(ctx, base64.StdEncoding.EncodeToString([]byte("random-code")))
	// We hijack the request and return an error intentionally
	require.Error(t, err, "error expected")
}
