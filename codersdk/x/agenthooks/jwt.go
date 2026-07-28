package agenthooks

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

const jwtType = "JWT"

// MinSecretLen is the minimum HS256 secret length in bytes. go-jose
// accepts shorter keys, so signing and verification enforce it to fail
// closed on missing or weak secrets.
const MinSecretLen = 32

// SignClaims signs claims with the shared secret using HS256.
func SignClaims(secret []byte, claims Claims) (string, error) {
	if len(secret) < MinSecretLen {
		return "", xerrors.Errorf("secret must be at least %d bytes", MinSecretLen)
	}
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.HS256, Key: secret},
		new(jose.SignerOptions).WithType(jwtType),
	)
	if err != nil {
		return "", xerrors.Errorf("create signer: %w", err)
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", xerrors.Errorf("marshal claims: %w", err)
	}
	signed, err := signer.Sign(payload)
	if err != nil {
		return "", xerrors.Errorf("sign claims: %w", err)
	}
	token, err := signed.CompactSerialize()
	if err != nil {
		return "", xerrors.Errorf("serialize token: %w", err)
	}
	return token, nil
}

// Verify authenticates an HS256 bearer token and validates its JWT header,
// required claims, and validity window. Request binding remains the caller's
// responsibility; see NewHTTPHandler.
func Verify(authzHeader string, secret []byte) (Claims, error) {
	const bearerPrefix = "Bearer "
	token, ok := strings.CutPrefix(authzHeader, bearerPrefix)
	if !ok || token == "" || strings.ContainsAny(token, " \t\r\n") {
		return Claims{}, xerrors.New("authorization header must contain one Bearer token")
	}
	return verifyToken(token, secret)
}

// verifyToken authenticates a compact HS256 token and validates its JWT
// header, required claims, and validity window.
func verifyToken(token string, secret []byte) (Claims, error) {
	if len(secret) < MinSecretLen {
		return Claims{}, xerrors.Errorf("secret must be at least %d bytes", MinSecretLen)
	}
	if token == "" || strings.ContainsAny(token, " \t\r\n") {
		return Claims{}, xerrors.New("token must be one compact JWT")
	}

	object, err := jose.ParseSigned(token, []jose.SignatureAlgorithm{jose.HS256})
	if err != nil {
		return Claims{}, xerrors.Errorf("parse token: %w", err)
	}
	if len(object.Signatures) != 1 {
		return Claims{}, xerrors.New("token must contain one signature")
	}
	header := object.Signatures[0].Header
	typ, ok := header.ExtraHeaders[jose.HeaderType].(string)
	if !ok || typ != jwtType {
		return Claims{}, xerrors.Errorf("token type must be %q", jwtType)
	}

	payload, err := object.Verify(secret)
	if err != nil {
		return Claims{}, xerrors.Errorf("verify token: %w", err)
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, xerrors.Errorf("decode claims: %w", err)
	}
	if err := validateClaims(claims, time.Now()); err != nil {
		return Claims{}, err
	}
	return claims, nil
}

// responseTokenTTL bounds a response token's validity. The dispatcher reads
// the response within its own dispatch timeout, so the window only needs to
// absorb clock skew between coderd and the consumer.
const responseTokenTTL = 2 * time.Minute

// responseClockSkewLeeway backdates nbf so a consumer clock slightly ahead
// of coderd's does not produce tokens that read as not yet valid.
const responseClockSkewLeeway = 30 * time.Second

// SignResponse mints the SignatureHeader token for one hook response: the
// verified request claims echoed with body_sha256 recomputed over the exact
// bytes of responseBody and a fresh validity window. Echoing jti binds the
// response to its dispatch, so a captured response cannot answer any other
// dispatch even within the validity window.
func SignResponse(secret []byte, requestClaims Claims, responseBody []byte) (string, error) {
	digest := sha256.Sum256(responseBody)
	now := time.Now()
	claims := requestClaims
	claims.BodySHA256 = hex.EncodeToString(digest[:])
	claims.IssuedAt = now.Unix()
	claims.NotBefore = now.Add(-responseClockSkewLeeway).Unix()
	claims.Expires = now.Add(responseTokenTTL).Unix()
	return SignClaims(secret, claims)
}

// VerifyResponse authenticates a hook response token and binds it to the
// dispatch identified by requestClaims and to the exact response body bytes.
func VerifyResponse(token string, secret []byte, requestClaims Claims, responseBody []byte) error {
	if token == "" {
		return xerrors.Errorf("response carries no %s header", SignatureHeader)
	}
	claims, err := verifyToken(token, secret)
	if err != nil {
		return err
	}
	switch {
	case claims.JTI != requestClaims.JTI:
		return xerrors.New("response JWT ID does not match the dispatch")
	case claims.Type != requestClaims.Type:
		return xerrors.New("response type claim does not match the dispatched event")
	case claims.Subject != requestClaims.Subject:
		return xerrors.New("response subject claim does not match the dispatched chat")
	case claims.Audience != requestClaims.Audience:
		return xerrors.New("response audience claim does not match the hook URL")
	}
	digest := sha256.Sum256(responseBody)
	if claims.BodySHA256 != hex.EncodeToString(digest[:]) {
		return xerrors.New("response body does not match body_sha256 claim")
	}
	return nil
}

func validateClaims(claims Claims, now time.Time) error {
	switch {
	case claims.Issuer == "":
		return xerrors.New("issuer is required")
	case claims.Subject == "":
		return xerrors.New("subject is required")
	case claims.Audience == "":
		return xerrors.New("audience is required")
	case claims.IssuedAt == 0:
		return xerrors.New("issued at is required")
	case claims.NotBefore == 0:
		return xerrors.New("not before is required")
	case claims.Expires == 0:
		return xerrors.New("expiry is required")
	case claims.JTI == uuid.Nil:
		return xerrors.New("JWT ID is required")
	case !validEventType(claims.Type):
		return xerrors.Errorf("invalid event type %q", claims.Type)
	case !validSHA256(claims.BodySHA256):
		return xerrors.New("body SHA-256 must be a hexadecimal SHA-256 digest")
	case claims.NotBefore > claims.Expires:
		return xerrors.New("not before must not be after expiry")
	case claims.IssuedAt > claims.Expires:
		return xerrors.New("issued at must not be after expiry")
	case now.Unix() < claims.NotBefore:
		return xerrors.New("token is not valid yet")
	case now.Unix() >= claims.Expires:
		return xerrors.New("token has expired")
	}
	if _, err := claims.ChatID(); err != nil {
		return err
	}
	return nil
}

func validEventType(eventType EventType) bool {
	switch eventType {
	case EventSessionStart, EventUserPromptSubmit, EventPreToolUse,
		EventPostToolUse, EventPreCompact, EventPostCompact, EventStop:
		return true
	default:
		return false
	}
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}
