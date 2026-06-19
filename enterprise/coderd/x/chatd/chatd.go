package chatd

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	osschatd "github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/websocket"
)

// RelaySourceHeader marks replica-relayed stream requests.
const RelaySourceHeader = "X-Coder-Relay-Source-Replica"

const (
	authorizationHeader = "Authorization"
	cookieHeader        = "Cookie"
)

// RelayDialError wraps a failed relay handshake. HTTPStatus is 0
// when the failure happened before a response.
type RelayDialError struct {
	HTTPStatus int
	Err        error
}

func (e *RelayDialError) Error() string { return e.Err.Error() }
func (e *RelayDialError) Unwrap() error { return e.Err }

// IsUnrecoverable reports whether retrying with the same captured
// session token is futile.
func (e *RelayDialError) IsUnrecoverable() bool {
	return e.HTTPStatus == http.StatusUnauthorized ||
		e.HTTPStatus == http.StatusForbidden
}

// StreamPartsDialerConfig holds dependencies for multi-replica stream parts.
type StreamPartsDialerConfig struct {
	ResolveReplicaAddress func(context.Context, uuid.UUID) (string, bool)
	ReplicaHTTPClient     *http.Client
	ReplicaIDFn           func() uuid.UUID
	DialerFn              func(context.Context, osschatd.StreamPartsDialInput) (osschatd.StreamPartsSession, error)
}

// NewStreamPartsDialer returns a dialer for the owning replica's parts endpoint.
func NewStreamPartsDialer(cfg StreamPartsDialerConfig) osschatd.StreamPartsDialer {
	return func(ctx context.Context, input osschatd.StreamPartsDialInput) (osschatd.StreamPartsSession, error) {
		if cfg.DialerFn != nil {
			return cfg.DialerFn(ctx, input)
		}
		return dialRelayParts(ctx, input, cfg)
	}
}

func dialRelayParts(
	ctx context.Context,
	input osschatd.StreamPartsDialInput,
	cfg StreamPartsDialerConfig,
) (osschatd.StreamPartsSession, error) {
	if cfg.ResolveReplicaAddress == nil {
		return nil, &RelayDialError{Err: xerrors.New("dial relay stream parts: resolver not configured")}
	}
	address, ok := cfg.ResolveReplicaAddress(ctx, input.WorkerID)
	if !ok {
		return nil, &RelayDialError{Err: xerrors.New("dial relay stream parts: worker replica not found")}
	}
	wsURL, err := buildRelayURL(address, input.ChatID)
	if err != nil {
		return nil, &RelayDialError{Err: xerrors.Errorf("dial relay stream parts: %w", err)}
	}

	if cfg.ReplicaIDFn == nil {
		return nil, &RelayDialError{Err: xerrors.New("dial relay stream parts: replica ID function not configured")}
	}
	replicaID := cfg.ReplicaIDFn()
	if replicaID == uuid.Nil {
		return nil, &RelayDialError{Err: xerrors.New("dial relay stream parts: replica ID is nil")}
	}
	headers := make(http.Header, 2)
	headers.Set(codersdk.SessionTokenHeader, ExtractSessionToken(input.RequestHeader))
	headers.Set(RelaySourceHeader, replicaID.String())

	conn, resp, dialErr := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPClient:      cfg.ReplicaHTTPClient,
		HTTPHeader:      headers,
		CompressionMode: websocket.CompressionDisabled,
	})
	status := 0
	if resp != nil {
		status = resp.StatusCode
		if dialErr != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}
	if dialErr != nil {
		return nil, &RelayDialError{
			HTTPStatus: status,
			Err:        xerrors.Errorf("dial relay stream parts: %w", dialErr),
		}
	}
	conn.SetReadLimit(1 << 22)
	return osschatd.NewStreamPartsJSONSession(ctx, conn), nil
}

// buildRelayURL builds the websocket URL for the chat stream parts endpoint on
// a peer replica. It maps http(s) schemes to ws(s).
func buildRelayURL(address string, chatID uuid.UUID) (string, error) {
	u, err := url.Parse(address)
	if err != nil {
		return "", xerrors.Errorf("parse relay address %q: %w", address, err)
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", xerrors.Errorf("unsupported relay address scheme %q", u.Scheme)
	}
	u.Path = "/api/experimental/chats/" + chatID.String() + "/stream/parts"
	u.RawQuery = ""
	return u.String(), nil
}

// ExtractSessionToken returns the session token carried by the given request
// headers, mirroring the priority order used by apiKeyMiddleware: cookie,
// then Coder-Session-Token header, then Authorization: Bearer header.
func ExtractSessionToken(header http.Header) string {
	if header == nil {
		return ""
	}
	if raw := header.Get(cookieHeader); raw != "" {
		r := &http.Request{Header: http.Header{cookieHeader: {raw}}}
		if c, err := r.Cookie(codersdk.SessionTokenCookie); err == nil && c.Value != "" {
			return c.Value
		}
	}
	if v := header.Get(codersdk.SessionTokenHeader); v != "" {
		return v
	}
	if v := header.Get(authorizationHeader); len(v) > 7 && strings.EqualFold(v[:7], "bearer ") {
		return strings.TrimSpace(v[7:])
	}
	return ""
}
