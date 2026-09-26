package routing

import (
	"context"
	"fmt"
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
)

// MaxRequestBodyBytes caps the request body size for AI Gateway
// provider endpoints to prevent denial-of-service via memory exhaustion.
// Anthropic enforces 32 MB on the direct API, 30 MB on Vertex AI,
// and 20 MB on Amazon Bedrock.
// See https://docs.anthropic.com/en/api/overview#request-size-limits
// OpenAI and GitHub Copilot do not document an equivalent HTTP body size limit.
// Using highest documented provider limit (32 MiB).
//
// NOTE: aibridge does not currently proxy file-upload endpoints
// (e.g. /v1/files). Those endpoints accept much larger bodies
// (up to 500 MB for Anthropic, 50 MB for OpenAI). If file-upload
// routes are added, they will need a per-route limit instead of
// this single global cap.
const MaxRequestBodyBytes = 32 << 20 // 32 MiB

// WriteRequestBodyTooLarge writes a human-readable 413 response indicating that
// the request body exceeded [MaxRequestBodyBytes].
//
// It records the limit before writing, so the request log names the limit that
// tripped and the too-large metric attributes the rejection to body size rather
// than to the other reasons coderd answers 413. Recording here rather than at
// each call site keeps the two inseparable: this helper is the only path to a
// body-too-large response from aibridge.
func WriteRequestBodyTooLarge(ctx context.Context, w http.ResponseWriter) {
	httpapi.RecordRequestBodyLimit(ctx, MaxRequestBodyBytes)
	http.Error(w, fmt.Sprintf(
		"Request body too large. The maximum allowed request body size is %dMiB.",
		MaxRequestBodyBytes>>20,
	), http.StatusRequestEntityTooLarge)
}
