package aibridge

import (
	"net/http"

	"github.com/coder/coder/v2/aibridge/clientmeta"
)

// GuessSessionID attempts to retrieve a session ID sent by the client.
func GuessSessionID(client Client, r *http.Request) *string {
	return clientmeta.GuessSessionID(client, r)
}

// GuessSessionIDFromPayload retrieves a session ID without reading r.Body.
func GuessSessionIDFromPayload(client Client, r *http.Request, payload []byte) *string {
	return clientmeta.GuessSessionIDFromPayload(client, r, payload)
}
