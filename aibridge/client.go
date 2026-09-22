package aibridge

import (
	"net/http"

	"github.com/coder/coder/v2/aibridge/clientmeta"
)

type Client = clientmeta.Client

const (
	ClientClaudeCode  = clientmeta.ClientClaudeCode
	ClientCodex       = clientmeta.ClientCodex
	ClientZed         = clientmeta.ClientZed
	ClientCopilotVSC  = clientmeta.ClientCopilotVSC
	ClientCopilotCLI  = clientmeta.ClientCopilotCLI
	ClientKilo        = clientmeta.ClientKilo
	ClientCoderAgents = clientmeta.ClientCoderAgents
	ClientCrush       = clientmeta.ClientCrush
	ClientXum         = clientmeta.ClientXum
	ClientRoo         = clientmeta.ClientRoo
	ClientCursor      = clientmeta.ClientCursor
	ClientOpenCode    = clientmeta.ClientOpenCode
	ClientJunie       = clientmeta.ClientJunie
	ClientUnknown     = clientmeta.ClientUnknown
)

// GuessClient attempts to guess the client application from the request headers.
// Not all clients set proper user agent headers, so this is a best-effort approach.
// Based on https://github.com/coder/aibridge/issues/20#issuecomment-3769444101.
func GuessClient(r *http.Request) Client {
	return clientmeta.GuessClient(r)
}
