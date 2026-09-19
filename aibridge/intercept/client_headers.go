package intercept

import (
	"net/http"
	"net/textproto"
	"strings"

	"github.com/google/uuid"
)

// ChatIDPlaceholder is the template token an admin may embed in a custom
// upstream header value to get per-conversation stability (e.g.
// x-opencode-session: {{chat_id}} for OpenCode Zen routing). It must match
// codersdk.ChatIDPlaceholder; it is redefined here because aibridge does
// not import codersdk.
const ChatIDPlaceholder = "{{chat_id}}"

// chatIDHeader names the conversation header Coder Agents sends on gateway
// requests. It must match chatprovider.HeaderCoderChatID; it is redefined
// here because aibridge cannot import chatd (coderd imports aibridge).
const chatIDHeader = "X-Coder-Chat-Id"

// hopByHopHeaders are connection-level headers specific to the connection
// between client and AI Gateway, not meant for the upstream.
// See https://www.rfc-editor.org/rfc/rfc2616#section-13.5.1
var hopByHopHeaders = []string{
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
var nonForwardedHeaders = []string{
	"Host",
	"Accept-Encoding",
	"Content-Length",
}

// authHeaders are headers that carry authentication credentials from the
// client. The upstream request is built by the SDK, which sets the correct
// provider credentials via option.WithAPIKey. Client auth headers are
// stripped here and the provider credentials are re-injected by
// BuildUpstreamHeaders from the SDK-built request.
var authHeaders = []string{
	"Authorization",
	"X-Api-Key",
}

// proxyHeaders describe the path the inbound request took to reach
// aibridge. On bridge routes aibridge acts as a client, not a proxy,
// so these headers are not meaningful on the outbound request.
var proxyHeaders = []string{
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
var agentFirewallHeaders = []string{
	"X-Coder-Agent-Firewall-Session-Id",
	"X-Coder-Agent-Firewall-Sequence-Number",
}

// PrepareClientHeaders returns a copy of the client headers with hop-by-hop,
// transport, auth, and proxy headers removed. A nil input yields an empty
// (non-nil) header set so callers can unconditionally set preserved headers.
func PrepareClientHeaders(clientHeaders http.Header) http.Header {
	prepared := clientHeaders.Clone()
	if prepared == nil {
		prepared = make(http.Header)
	}
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
	return prepared
}

// BuildUpstreamHeaders produces the header set for an upstream SDK request.
// It starts from the prepared client headers, then preserves specific
// headers from the SDK-built request that must not be overwritten.
func BuildUpstreamHeaders(sdkHeader http.Header, clientHeaders http.Header, authHeaderName string) http.Header {
	headers := PrepareClientHeaders(clientHeaders)

	// Preserve the auth header set by the SDK from the provider configuration.
	if v := sdkHeader.Get(authHeaderName); v != "" {
		headers.Set(authHeaderName, v)
	}

	// Preserve actor headers injected by aibridge as per-request SDK options.
	for name, values := range sdkHeader {
		if IsActorHeader(name) {
			headers[name] = values
		}
	}

	return headers
}

// ApplyUpstreamHeaders sets admin-configured custom headers on dst, the
// outbound upstream request headers. Configured headers win over any
// same-named header forwarded from the client. dst must be non-nil; a nil
// clientHeaders reads as absent.
//
// A value embedding ChatIDPlaceholder resolves per request to the caller's
// conversation ID (chatIDHeader) when reachable, so upstreams that key
// routing off a session header see a stable value per conversation. When
// the conversation ID is absent, the placeholder resolves to a
// deployment-stable UUID derived from the provider name, so requests still
// carry a consistent session value across gateway restarts. Values without
// the placeholder are sent verbatim.
func ApplyUpstreamHeaders(dst http.Header, configured map[string]string, clientHeaders http.Header, providerName string) {
	if len(configured) == 0 {
		return
	}
	chatID := strings.TrimSpace(clientHeaders.Get(chatIDHeader))
	for name, value := range configured {
		resolved := value
		if strings.Contains(resolved, ChatIDPlaceholder) {
			session := chatID
			if session == "" {
				session = fallbackSessionID(providerName)
			}
			resolved = strings.ReplaceAll(resolved, ChatIDPlaceholder, session)
		}
		dst.Set(textproto.CanonicalMIMEHeaderKey(name), resolved)
	}
}

// fallbackSessionID derives a deployment-stable session UUID from the
// provider name. It is deterministic (SHA-1 over a fixed namespace) so the
// value survives gateway restarts and reloads, unlike a random UUID minted
// per process.
func fallbackSessionID(providerName string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("coder:aibridge:upstream-headers:"+providerName)).String()
}
