package aibridge

import (
	"net/http"

	"github.com/google/uuid"
)

// TransportFactory returns an [http.RoundTripper] that routes a chatd LLM
// request through aibridge in-process, for a given ai_providers row.
//
// Implementations live in coderd/aibridged. coderd registers an in-process
// factory on coderd.API.AIBridgeTransportFactory at startup so chatd routes
// LLM traffic through the daemon without going through the gated HTTP route.
//
// TransportFor returns (nil, nil) when the caller should fall through to
// direct upstream behavior, e.g. when the request is not coder-agent
// traffic and the licensing carve-out does not apply.
type TransportFactory interface {
	TransportFor(providerID uuid.UUID, isCoderAgent bool) (http.RoundTripper, error)
}
