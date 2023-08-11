package oauthpki

import (
	"context"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"
	"time"

	"github.com/coder/coder/coderd/httpmw"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"
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
	hashed := sha1.Sum(block.Bytes)

	return &Config{
		clientID:  params.ClientID,
		tokenURL:  params.TokenURL,
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
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": ja.clientID,
		"sub": ja.clientID,
		"aud": ja.tokenURL,
		"exp": now.Add(time.Minute * 5).Unix(),
		"jti": uuid.New().String(),
		"nbf": now.Unix(),
		"iat": now.Unix(),
	})
	token.Header["x5t"] = ja.x5t

	signed, err := token.SignedString(ja.clientKey)
	if err != nil {
		return nil, xerrors.Errorf("failed to sign jwt assertion: %w", err)
	}

	opts = append(opts,
		oauth2.SetAuthURLParam("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"),
		oauth2.SetAuthURLParam("client_assertion", signed),
	)
	return ja.cfg.Exchange(ctx, code, opts...)
}

func (ja *Config) TokenSource(ctx context.Context, token *oauth2.Token) oauth2.TokenSource {
	return ja.cfg.TokenSource(ctx, token)
}
