package aibridge

import "golang.org/x/xerrors"

// Delegation headers are reserved for authenticated infrastructure callers.
const (
	HeaderGatewayKey         = "X-Coder-AI-Gateway-Key"
	HeaderDelegatedAPIKeyID  = "X-Coder-AI-Gateway-Delegated-Key-ID" //nolint:gosec // HTTP header name, not a credential.
	HeaderDelegatedSource    = "X-Coder-AI-Gateway-Source"
	HeaderDelegatedWorkspace = "X-Coder-AI-Gateway-Workspace-ID"
	HeaderGatewayError       = "X-Coder-AI-Gateway-Error"
	GatewayKeyMismatchCode   = "gateway_key_mismatch"
)

// ErrGatewayKeyMismatch identifies an infrastructure authentication rejection
// before provider dispatch. Only this Gateway authentication error is retryable.
var ErrGatewayKeyMismatch = xerrors.New("AI Gateway authentication failed: coderd and the standalone Gateway must be configured with the same Gateway key")
