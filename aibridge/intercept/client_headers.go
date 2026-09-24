package intercept

import (
	"net/http"

	"github.com/coder/coder/v2/aibridge/utils"
)

// PrepareClientHeaders returns a copy of the client headers with the headers
// listed by utils.StripClientRequestHeaders removed. The SDK sets the provider
// credential, which BuildUpstreamHeaders copies from the SDK-built request.
func PrepareClientHeaders(clientHeaders http.Header) http.Header {
	prepared := clientHeaders.Clone()
	utils.StripClientRequestHeaders(prepared)
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
