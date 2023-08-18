package oauthpki

import (
	"context"
	"crypto/rsa"
	"crypto/sha1" //#nosec // Not used for cryptography.
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/jws"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/httpmw"
)

// Config uses jwt assertions over client_secret for oauth2 authentication of
// the application. This implementation was made specifically for Azure AD.
//
//	https://learn.microsoft.com/en-us/azure/active-directory/develop/certificate-credentials
//
// However this does mostly follow the standard. We can generalize this as we
// include support for more IDPs.
//
//	https://datatracker.ietf.org/doc/html/rfc7523
type Config struct {
	cfg httpmw.OAuth2Config

	// These values should match those provided in the oauth2.Config.
	// Because the inner config is an interface, we need to duplicate these
	// values here.
	scopes   []string
	clientID string
	tokenURL string

	// ClientSecret is the private key of the PKI cert.
	// Azure AD only supports RS256 signing algorithm.
	clientKey *rsa.PrivateKey
	// Base64url-encoded SHA-1 thumbprint of the X.509 certificate's DER encoding.
	// This is specific to Azure AD
	x5t string
}

type ConfigParams struct {
	ClientID       string
	TokenURL       string
	Scopes         []string
	PemEncodedKey  []byte
	PemEncodedCert []byte

	Config httpmw.OAuth2Config
}

// NewOauth2PKIConfig creates the oauth2 config for PKI based auth. It requires the certificate and it's private key.
// The values should be passed in as PEM encoded values, which is the standard encoding for x509 certs saved to disk.
// It should look like:
//
// -----BEGIN RSA PRIVATE KEY----
// ...
// -----END RSA PRIVATE KEY-----
//
// -----BEGIN CERTIFICATE-----
// ...
// -----END CERTIFICATE-----
func NewOauth2PKIConfig(params ConfigParams) (*Config, error) {
	if params.ClientID == "" {
		return nil, xerrors.Errorf("")
	}
	if len(params.Scopes) == 0 {
		return nil, xerrors.Errorf("scopes are required")
	}

	rsaKey, err := decodeClientKey(params.PemEncodedKey)
	if err != nil {
		return nil, err
	}

	// Azure AD requires a certificate. The sha1 of the cert is used to identify the signer.
	// This is not required in the general specification.
	if strings.Contains(strings.ToLower(params.TokenURL), "microsoftonline") && len(params.PemEncodedCert) == 0 {
		return nil, xerrors.Errorf("oidc client certificate is required and missing")
	}

	block, _ := pem.Decode(params.PemEncodedCert)
	// Used as an identifier, not an actual cryptographic hash.
	//nolint:gosec
	hashed := sha1.Sum(block.Bytes)

	return &Config{
		clientID:  params.ClientID,
		tokenURL:  params.TokenURL,
		scopes:    params.Scopes,
		cfg:       params.Config,
		clientKey: rsaKey,
		x5t:       base64.StdEncoding.EncodeToString(hashed[:]),
	}, nil
}

// decodeClientKey decodes a PEM encoded rsa secret.
func decodeClientKey(pemEncoded []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemEncoded)
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, xerrors.Errorf("failed to parse private key: %w", err)
	}

	return key, nil
}

func (ja *Config) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	return ja.cfg.AuthCodeURL(state, opts...)
}

// Exchange includes the client_assertion signed JWT.
func (ja *Config) Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	signed, err := ja.jwtToken()
	if err != nil {
		return nil, xerrors.Errorf("failed jwt assertion: %w", err)
	}
	opts = append(opts,
		oauth2.SetAuthURLParam("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"),
		oauth2.SetAuthURLParam("client_assertion", signed),
	)
	return ja.cfg.Exchange(ctx, code, opts...)
}

func (ja *Config) jwtToken() (string, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": ja.clientID,
		"sub": ja.clientID,
		"aud": ja.tokenURL,
		// 5-10 minutes is recommended in the Azure docs.
		// So we'll use 5 minutes.
		"exp": now.Add(time.Minute * 5).Unix(),
		"jti": uuid.New().String(),
		"nbf": now.Unix(),
		"iat": now.Unix(),
	})
	token.Header["x5t"] = ja.x5t

	signed, err := token.SignedString(ja.clientKey)
	if err != nil {
		return "", xerrors.Errorf("sign jwt assertion: %w", err)
	}
	return signed, nil
}

func (ja *Config) TokenSource(ctx context.Context, token *oauth2.Token) oauth2.TokenSource {
	return oauth2.ReuseTokenSource(token, &jwtTokenSource{
		cfg:          ja,
		ctx:          ctx,
		refreshToken: token.RefreshToken,
	})
}

type jwtTokenSource struct {
	cfg          *Config
	ctx          context.Context
	refreshToken string
}

// Token must be safe for concurrent use by multiple go routines
// Very similar to the RetrieveToken implementation by the oauth2 package.
// https://github.com/golang/oauth2/blob/master/internal/token.go#L212
// Oauth2 package keeps this code unexported or in an /internal package,
// so we have to copy the implementation :(
func (src *jwtTokenSource) Token() (*oauth2.Token, error) {
	if src.refreshToken == "" {
		return nil, xerrors.New("oauth2: token expired and refresh token is not set")
	}
	cli := http.DefaultClient
	if v, ok := src.ctx.Value(oauth2.HTTPClient).(*http.Client); ok {
		cli = v
	}

	token, err := src.cfg.jwtToken()
	if err != nil {
		return nil, xerrors.Errorf("failed jwt assertion: %w", err)
	}

	v := url.Values{
		"client_assertion":      {token},
		"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
		"client_id":             {src.cfg.clientID},
		"grant_type":            {"refresh_token"},
		"scope":                 {strings.Join(src.cfg.scopes, " ")},
		"refresh_token":         {src.refreshToken},
	}
	// Using params based auth
	req, err := http.NewRequest("POST", src.cfg.tokenURL, strings.NewReader(v.Encode()))
	if err != nil {
		return nil, xerrors.Errorf("oauth2: make token refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(src.ctx)
	resp, err := cli.Do(req)
	if err != nil {
		return nil, xerrors.Errorf("oauth2: cannot get token: %w", err)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, xerrors.Errorf("oauth2: cannot fetch token reading response body: %w", err)
	}

	var tokenRes struct {
		oauth2.Token
		// Extra fields returned by the refresh that are needed
		IDToken   string `json:"id_token"`
		ExpiresIn int64  `json:"expires_in"` // relative seconds from now
		// error fields
		// https://datatracker.ietf.org/doc/html/rfc6749#section-5.2
		ErrorCode        string `json:"error"`
		ErrorDescription string `json:"error_description"`
		ErrorURI         string `json:"error_uri"`
	}

	unmarshalError := json.Unmarshal(body, &tokenRes)

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// Return a standard oauth2 error. Attempt to read some error fields. The error fields
		// can be encoded in a few places, so this does not catch all of them.
		return nil, &oauth2.RetrieveError{
			Response: resp,
			Body:     body,
			// Best effort for error fields
			ErrorCode:        tokenRes.ErrorCode,
			ErrorDescription: tokenRes.ErrorDescription,
			ErrorURI:         tokenRes.ErrorURI,
		}
	}

	if unmarshalError != nil {
		return nil, xerrors.Errorf("oauth2: cannot unmarshal token: %w", err)
	}

	newToken := &oauth2.Token{
		AccessToken:  tokenRes.AccessToken,
		TokenType:    tokenRes.TokenType,
		RefreshToken: tokenRes.RefreshToken,
	}

	if secs := tokenRes.ExpiresIn; secs > 0 {
		newToken.Expiry = time.Now().Add(time.Duration(secs) * time.Second)
	}

	// ID token is a JWT token. We can decode it to get the expiry.
	// Not really sure what to do if the ExpiresIn and JWT expiry differ,
	// but this one is attached in the JWT and guaranteed to be right for local
	// validation. So use this one if found.
	if v := tokenRes.IDToken; v != "" {
		// decode returned id token to get expiry
		claimSet, err := jws.Decode(v)
		if err != nil {
			return nil, xerrors.Errorf("oauth2: error decoding JWT token: %w", err)
		}
		newToken.Expiry = time.Unix(claimSet.Exp, 0)
	}

	return newToken, nil
}
