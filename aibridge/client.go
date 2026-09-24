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

// GuessClient attempts to identify the client application from request headers.
func GuessClient(r *http.Request) Client {
	return clientmeta.GuessClient(r)
}
