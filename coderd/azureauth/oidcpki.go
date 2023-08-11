package azureauth

import (
	"context"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"time"

	"github.com/google/uuid"

	"github.com/golang-jwt/jwt/v4"

	"golang.org/x/xerrors"

	"golang.org/x/oauth2"
)

// JWTAssertion is used by Azure AD when doing OIDC with a PKI cert instead of
// a client secret.
// https://learn.microsoft.com/en-us/azure/active-directory/develop/certificate-credentials
type JWTAssertion struct {
	*oauth2.Config

	// ClientSecret is the private key of the PKI cert.
	// Azure AD only supports RS256 signing algorithm.
	clientKey *rsa.PrivateKey
	// Base64url-encoded SHA-1 thumbprint of the X.509 certificate's DER encoding.
	x5t string
}

func NewJWTAssertion(config *oauth2.Config, pemEncodedKey string, pemEncodedCert string) (*JWTAssertion, error) {
	rsaKey, err := DecodeKeyCertificate([]byte(pemEncodedKey))
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode([]byte(pemEncodedCert))
	hashed := sha1.Sum(block.Bytes)

	return &JWTAssertion{
		Config:    config,
		clientKey: rsaKey,
		x5t:       base64.StdEncoding.EncodeToString(hashed[:]),
	}, nil
}

// DecodeKeyCertificate decodes a PEM encoded PKI cert.
func DecodeKeyCertificate(pemEncoded []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemEncoded)
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, xerrors.Errorf("failed to parse private key: %w", err)
	}

	return key, nil
}

func (ja *JWTAssertion) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	return ja.Config.AuthCodeURL(state, opts...)
}

func (ja *JWTAssertion) Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"aud": ja.Config.Endpoint.TokenURL,
		"exp": now.Add(time.Minute * 5).Unix(),
		"jti": uuid.New().String(),
		"nbf": now.Unix(),
		"iat": now.Unix(),

		// TODO: Should be app GUID, not client ID.
		"iss": ja.Config.ClientID,
		"sub": ja.Config.ClientID,
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
	return ja.Config.Exchange(ctx, code, opts...)
}

func (ja *JWTAssertion) TokenSource(ctx context.Context, token *oauth2.Token) oauth2.TokenSource {
	return ja.Config.TokenSource(ctx, token)
}
