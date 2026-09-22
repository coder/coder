package aibridge

import (
	"net/http"

	"github.com/coder/coder/v2/aibridge/clientmeta"
)

// GuessSessionID attempts to retrieve a session ID which may have been sent by
// the client. We only attempt to retrieve sessions using methods recognized for
// the given client.
func GuessSessionID(client Client, r *http.Request) *string {
	return clientmeta.GuessSessionID(client, r)
}
