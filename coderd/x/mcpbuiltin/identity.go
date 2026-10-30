package mcpbuiltin

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// Header names of Coder's forwarded identity headers and their signature.
// They match what chatd sends to MCP servers configured with "Forward Coder
// headers" and a signing secret.
const (
	HeaderCoderOwnerID            = "X-Coder-Owner-Id"
	HeaderCoderChatID             = "X-Coder-Chat-Id"
	HeaderCoderSubchatID          = "X-Coder-Subchat-Id"
	HeaderCoderWorkspaceID        = "X-Coder-Workspace-Id"
	HeaderCoderSignatureTimestamp = "X-Coder-Signature-Timestamp"
	HeaderCoderSignature          = "X-Coder-Signature"
)

// signatureMaxSkew is how far a signature timestamp may be from now.
const signatureMaxSkew = 5 * time.Minute

// Identity is the verified Coder identity of an MCP request.
type Identity struct {
	OwnerID uuid.UUID
	// ChatID is the root chat. For a subchat this is its parent, which
	// is the chat that a trigger conversation maps to.
	ChatID      uuid.UUID
	SubchatID   uuid.NullUUID
	WorkspaceID uuid.NullUUID
}

// IdentityVerifier authenticates Coder's signed identity headers.
type IdentityVerifier interface {
	// VerifyIdentity returns the identity of a request carrying an
	// X-Coder-Signature header, or an error when the signature is
	// missing, stale, or does not match. server is the built-in server
	// name, such as ServerSlack. body is the full request body.
	VerifyIdentity(ctx context.Context, server string, r *http.Request, body []byte) (Identity, error)
}

// SigningSecretsFunc returns the signing secrets of the MCP server
// configurations that may have signed a request to the named built-in
// server on behalf of chatID.
type SigningSecretsFunc func(ctx context.Context, server string, chatID uuid.UUID) ([]string, error)

// HMACVerifier verifies the v1 HMAC-SHA256 signature chatd adds to
// forwarded identity headers.
type HMACVerifier struct {
	Secrets SigningSecretsFunc
	// Now returns the current time. Defaults to time.Now.
	Now func() time.Time
}

var _ IdentityVerifier = HMACVerifier{}

// VerifyIdentity implements IdentityVerifier.
func (v HMACVerifier) VerifyIdentity(ctx context.Context, server string, r *http.Request, body []byte) (Identity, error) {
	signature := r.Header.Get(HeaderCoderSignature)
	timestamp := r.Header.Get(HeaderCoderSignatureTimestamp)
	if signature == "" || timestamp == "" {
		return Identity{}, xerrors.New("missing signature headers")
	}
	unix, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return Identity{}, xerrors.Errorf("parse signature timestamp: %w", err)
	}
	now := time.Now
	if v.Now != nil {
		now = v.Now
	}
	if now().Sub(time.Unix(unix, 0)).Abs() > signatureMaxSkew {
		return Identity{}, xerrors.New("signature timestamp is outside the allowed window")
	}

	identity, err := parseIdentity(r)
	if err != nil {
		return Identity{}, err
	}
	if v.Secrets == nil {
		return Identity{}, xerrors.New("no signing secrets are configured")
	}
	secrets, err := v.Secrets(ctx, server, identity.ChatID)
	if err != nil {
		return Identity{}, xerrors.Errorf("look up signing secrets: %w", err)
	}
	canonical := signatureCanonical(r, timestamp, body)
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		if hmac.Equal([]byte(sign(secret, canonical)), []byte(signature)) {
			return identity, nil
		}
	}
	return Identity{}, xerrors.New("signature does not match any MCP server configured for this chat")
}

func parseIdentity(r *http.Request) (Identity, error) {
	var (
		identity Identity
		err      error
	)
	identity.OwnerID, err = uuid.Parse(r.Header.Get(HeaderCoderOwnerID))
	if err != nil {
		return Identity{}, xerrors.Errorf("parse %s: %w", HeaderCoderOwnerID, err)
	}
	identity.ChatID, err = uuid.Parse(r.Header.Get(HeaderCoderChatID))
	if err != nil {
		return Identity{}, xerrors.Errorf("parse %s: %w", HeaderCoderChatID, err)
	}
	for header, dst := range map[string]*uuid.NullUUID{
		HeaderCoderSubchatID:   &identity.SubchatID,
		HeaderCoderWorkspaceID: &identity.WorkspaceID,
	} {
		raw := r.Header.Get(header)
		if raw == "" {
			continue
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			return Identity{}, xerrors.Errorf("parse %s: %w", header, err)
		}
		*dst = uuid.NullUUID{UUID: id, Valid: true}
	}
	return identity, nil
}

// signatureCanonical must stay byte-for-byte identical to the canonical
// form chatd signs in coderd/x/chatd/mcpclient.
func signatureCanonical(r *http.Request, timestamp string, body []byte) string {
	bodyHash := sha256.Sum256(body)
	method := r.Method
	if method == "" {
		method = http.MethodGet
	}
	return strings.Join([]string{
		"v1",
		timestamp,
		strings.ToUpper(method),
		r.URL.RequestURI(),
		hex.EncodeToString(bodyHash[:]),
		"owner=" + r.Header.Get(HeaderCoderOwnerID),
		"chat=" + r.Header.Get(HeaderCoderChatID),
		"subchat=" + r.Header.Get(HeaderCoderSubchatID),
		"workspace=" + r.Header.Get(HeaderCoderWorkspaceID),
	}, "\n")
}

func sign(secret, canonical string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}
