// Package headers defines HTTP header names and helpers shared by AI Gateway
// request handlers.
package headers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/net/http/httpguts"
	"golang.org/x/xerrors"

	aibcontext "github.com/coder/coder/v2/aibridge/context"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
)

const (
	// ActorAttributeID is the actor-header mapping key for the authenticated user ID.
	ActorAttributeID = "id"
	// ActorAttributeUsername is the actor-header mapping key for the username.
	ActorAttributeUsername = "username"

	// ActorHeaderPrefix prefixes every AI Bridge actor header.
	ActorHeaderPrefix      = "X-AI-Bridge-Actor"
	actorHeaderPrefixLower = "x-ai-bridge-actor"

	// AuthHeaderXAPIKey carries an API key.
	AuthHeaderXAPIKey = "X-Api-Key" //nolint:gosec // HTTP header name, not a credential.
	// AuthHeaderAuthorization carries an authorization credential.
	AuthHeaderAuthorization = "Authorization"

	// HeaderAnthropicWorkspaceID identifies the Anthropic workspace a request is
	// attributed to. Claude Platform for AWS requires it on every data plane
	// request. It is set from provider configuration, never preserved from the
	// client, so a client cannot choose which workspace its traffic bills to.
	HeaderAnthropicWorkspaceID = "Anthropic-Workspace-Id"
)

var (
	// hopByHopHeaders are connection-level headers specific to the connection
	// between client and AI Gateway, not meant for the upstream.
	// See https://www.rfc-editor.org/rfc/rfc2616#section-13.5.1
	hopByHopHeaders = []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
	}

	// nonForwardedHeaders are transport-level headers managed by aibridge or
	// Go's HTTP transport that must not be forwarded to the upstream provider.
	nonForwardedHeaders = []string{
		"Host",
		"Accept-Encoding",
		"Content-Length",
	}

	// authHeaders are headers that carry authentication credentials from the
	// client. The upstream request is built by the SDK, which sets the correct
	// provider credentials via option.WithAPIKey. Client auth headers are
	// stripped here and the provider credentials are re-injected by
	// BuildUpstreamHeaders from the SDK-built request.
	authHeaders = []string{
		"Authorization",
		"X-Api-Key",
	}

	// proxyHeaders describe the path the inbound request took to reach
	// aibridge. On bridge routes aibridge acts as a client, not a proxy,
	// so these headers are not meaningful on the outbound request.
	proxyHeaders = []string{
		"X-Forwarded-For",
		"X-Forwarded-Host",
		"X-Forwarded-Proto",
		"X-Forwarded-Port",
		"Forwarded",
	}

	// agentFirewallHeaders carry Agent Firewall correlation data used by
	// AI Gateway for session correlation. AI Gateway records the values
	// from the incoming request and strips the headers here so they are
	// never forwarded to upstream LLM providers.
	agentFirewallHeaders = []string{
		"X-Coder-Agent-Firewall-Session-Id",
		"X-Coder-Agent-Firewall-Sequence-Number",
	}
)

// ActorIDHeader returns the name of the header that carries the actor ID.
func ActorIDHeader() string {
	return fmt.Sprintf("%s-ID", ActorHeaderPrefix)
}

// ActorMetadataHeader returns the name of the header that carries the actor
// metadata value for name.
func ActorMetadataHeader(name string) string {
	return fmt.Sprintf("%s-Metadata-%s", ActorHeaderPrefix, name)
}

// IsActorHeader reports whether name is an AI Bridge actor header.
func IsActorHeader(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), actorHeaderPrefixLower)
}

// headersFromActor maps supported actor attributes to configured header names.
// Attributes with no destination and unknown mapping keys are ignored.
// No headers are returned if actor is nil.
func headersFromActor(actor *aibcontext.Actor, actorHeaderNames map[string]string) map[string]string {
	if actor == nil {
		return nil
	}

	headers := make(map[string]string, len(actorHeaderNames))
	if name := actorHeaderNames[ActorAttributeID]; name != "" {
		headers[name] = actor.ID
	}
	name := actorHeaderNames[ActorAttributeUsername]
	if name == "" {
		return headers
	}
	if username, ok := actor.Metadata["Username"].(string); ok && username != "" {
		headers[name] = username
	}

	return headers
}

// ExtractBearerToken extracts the token from a "Bearer <token>" authorization header.
func ExtractBearerToken(auth string) string {
	if auth := strings.TrimSpace(auth); auth != "" {
		fields := strings.Fields(auth)
		if len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
			return fields[1]
		}
	}
	return ""
}

// IsWebSocketUpgrade reports whether r is a WebSocket opening handshake.
func IsWebSocketUpgrade(r *http.Request) bool {
	return r.Method == http.MethodGet &&
		httpguts.HeaderValuesContainsToken(r.Header.Values("Connection"), "upgrade") &&
		httpguts.HeaderValuesContainsToken(r.Header.Values("Upgrade"), "websocket")
}

// ExtractAgentFirewallHeaders reads and parses the Agent Firewall
// correlation headers from the request. Both headers must be present
// together with a valid UUID session ID and a non-negative int32
// sequence number, or both must be absent. Partial or malformed headers
// return an error so the caller can reject the request (fail closed).
func ExtractAgentFirewallHeaders(r *http.Request) (sessionID *string, seqNumber *int32, err error) {
	rawSessionID := r.Header.Get(agplaibridge.HeaderAgentFirewallSessionID)
	rawSeqNumber := r.Header.Get(agplaibridge.HeaderAgentFirewallSequenceNumber)

	hasSessionID := rawSessionID != ""
	hasSeqNumber := rawSeqNumber != ""

	switch {
	case !hasSessionID && !hasSeqNumber:
		// Neither header present; request did not traverse Agent Firewall.
		return nil, nil, nil
	case hasSessionID && !hasSeqNumber:
		return nil, nil, xerrors.Errorf("agent firewall session ID header present without sequence number")
	case !hasSessionID && hasSeqNumber:
		return nil, nil, xerrors.Errorf("agent firewall sequence number header present without session ID")
	}

	// Both headers present; validate the session ID is a UUID. Storing an
	// invalid value would silently drop the firewall correlation to NULL
	// downstream, so reject it here instead.
	if _, parseErr := uuid.Parse(rawSessionID); parseErr != nil {
		return nil, nil, xerrors.Errorf("invalid agent firewall session ID %q: %w", rawSessionID, parseErr)
	}

	// Parse the sequence number.
	n, err := strconv.ParseInt(rawSeqNumber, 10, 32)
	if err != nil {
		return nil, nil, xerrors.Errorf("invalid agent firewall sequence number %q: %w", rawSeqNumber, err)
	}
	if n < 0 {
		return nil, nil, xerrors.Errorf("invalid agent firewall sequence number %q: must be non-negative", rawSeqNumber)
	}

	n32 := int32(n)
	return &rawSessionID, &n32, nil
}

// PrepareClientHeaders returns a copy of the client headers with hop-by-hop,
// transport, auth, and proxy headers removed.
func PrepareClientHeaders(clientHeaders http.Header) http.Header {
	prepared := clientHeaders.Clone()
	for _, h := range hopByHopHeaders {
		prepared.Del(h)
	}
	for _, h := range nonForwardedHeaders {
		prepared.Del(h)
	}
	for _, h := range authHeaders {
		prepared.Del(h)
	}
	for _, h := range proxyHeaders {
		prepared.Del(h)
	}
	for _, h := range agentFirewallHeaders {
		prepared.Del(h)
	}
	// Never forward a client-supplied workspace ID: providers that need one set
	// it from their own configuration.
	prepared.Del(HeaderAnthropicWorkspaceID)
	return prepared
}

// BuildUpstreamHeaders produces the header set for an upstream SDK request.
// It starts from the prepared client headers, preserves provider auth, then
// applies identity from the authenticated request actor.
// A nil actorHeaderNames map leaves client identity headers unchanged. A non-nil
// map enables identity cleanup, even when no attributes are selected.
func BuildUpstreamHeaders(sdkHeader http.Header, clientHeaders http.Header, authHeaderName string, actorHeaderNames map[string]string, actor *aibcontext.Actor) http.Header {
	headers := PrepareClientHeaders(clientHeaders)
	if headers == nil {
		headers = make(http.Header)
	}

	// Preserve the auth header set by the SDK from the provider configuration.
	if v := sdkHeader.Get(authHeaderName); v != "" {
		headers.Set(authHeaderName, v)
	}

	if actorHeaderNames == nil {
		return headers
	}
	for _, name := range []string{ActorIDHeader(), ActorMetadataHeader("Username")} {
		headers.Del(name)
	}
	for _, name := range actorHeaderNames {
		headers.Del(name)
	}
	for name, value := range headersFromActor(actor, actorHeaderNames) {
		headers.Set(name, value)
	}
	return headers
}
