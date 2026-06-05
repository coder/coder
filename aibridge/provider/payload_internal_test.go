package provider

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/intercept"
)

// reqPayload reads r's body into an intercept.Payload and restores the body so
// the request remains usable. Providers consume the payload instead of reading
// r.Body in CreateInterceptor.
func reqPayload(tb testing.TB, r *http.Request) intercept.Payload {
	tb.Helper()
	b, err := io.ReadAll(r.Body)
	require.NoError(tb, err)
	r.Body = io.NopCloser(bytes.NewReader(b))
	return intercept.NewPayload(b)
}
