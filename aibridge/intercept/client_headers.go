package intercept

import (
	"net/http"
)

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

// workspaceIDHeader is the Claude Platform for AWS workspace header. The SDK
// sets it from provider configuration; BuildUpstreamHeaders preserves it so it
// survives the client-header rebuild and is covered by the SigV4 signature.
const workspaceIDHeader = "Anthropic-Workspace-Id"

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

	// Preserve the Anthropic workspace ID set by the SDK from provider
	// configuration (Claude Platform for AWS). It is required on every data
	// plane request and, in SigV4 mode, is covered by the signature computed
	// after this rebuild. Harmless for providers that never set it.
	if v := sdkHeader.Get(workspaceIDHeader); v != "" {
		headers.Set(workspaceIDHeader, v)
	}

	// Preserve actor headers injected by aibridge as per-request SDK options.
	for name, values := range sdkHeader {
		if IsActorHeader(name) {
			headers[name] = values
		}
	}

	return headers
}
