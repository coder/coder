package aibridged

import (
	"bufio"
	"crypto/sha256"
	"crypto/subtle"
	"net"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/aibridge"
)

// DelegationMiddleware authenticates infrastructure callers using the same key
// the standalone Gateway uses to connect to coderd. Authenticated callers are
// trusted to supply the delegated identity; the handler still checks its validity.
// Requests without delegation headers retain ordinary client authentication.
func DelegationMiddleware(key string) func(http.Handler) http.Handler {
	expected := sha256.Sum256([]byte(key))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Header.Del(aibridge.HeaderGatewayError)
			headers := []string{
				aibridge.HeaderGatewayKey, aibridge.HeaderDelegatedAPIKeyID,
				aibridge.HeaderDelegatedSource, aibridge.HeaderDelegatedWorkspace,
			}
			delegated := false
			for _, name := range headers {
				if _, present := r.Header[http.CanonicalHeaderKey(name)]; present {
					delegated = true
				}
			}
			if !delegated {
				next.ServeHTTP(&gatewayResponseWriter{ResponseWriter: w}, r)
				return
			}

			provided := sha256.Sum256([]byte(r.Header.Get(aibridge.HeaderGatewayKey)))
			if key == "" || len(r.Header.Values(aibridge.HeaderGatewayKey)) != 1 || subtle.ConstantTimeCompare(expected[:], provided[:]) != 1 {
				w.Header().Set(aibridge.HeaderGatewayError, aibridge.GatewayKeyMismatchCode)
				http.Error(w, aibridge.ErrGatewayKeyMismatch.Error(), http.StatusUnauthorized)
				return
			}
			id := r.Header.Get(aibridge.HeaderDelegatedAPIKeyID)
			source := r.Header.Get(aibridge.HeaderDelegatedSource)
			workspace := r.Header.Get(aibridge.HeaderDelegatedWorkspace)
			if id == "" || strings.TrimSpace(id) != id || source == "" || strings.TrimSpace(source) != source ||
				len(r.Header.Values(aibridge.HeaderDelegatedAPIKeyID)) != 1 ||
				len(r.Header.Values(aibridge.HeaderDelegatedSource)) != 1 ||
				len(r.Header.Values(aibridge.HeaderDelegatedWorkspace)) > 1 {
				http.Error(w, "invalid AI Gateway delegation metadata", http.StatusBadRequest)
				return
			}
			attr := aibridge.Attribution{}
			if workspace != "" {
				id, err := uuid.Parse(workspace)
				if err != nil {
					http.Error(w, "invalid AI Gateway workspace attribution", http.StatusBadRequest)
					return
				}
				attr.WorkspaceID = id
			}
			ctx := aibridge.WithDelegatedAPIKeyID(r.Context(), id)
			ctx = aibridge.WithDelegatedAttribution(ctx, attr)
			ctx = aibridge.WithSource(ctx, aibridge.Source(source))
			r = r.Clone(ctx)
			for _, name := range headers {
				r.Header.Del(name)
			}
			next.ServeHTTP(&gatewayResponseWriter{ResponseWriter: w}, r)
		})
	}
}

// Only this middleware may emit the infrastructure error marker. In particular,
// an upstream provider's authentication error must never trigger delegation retries.
type gatewayResponseWriter struct {
	http.ResponseWriter
}

func (w *gatewayResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *gatewayResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

func (w *gatewayResponseWriter) WriteHeader(status int) {
	w.Header().Del(aibridge.HeaderGatewayError)
	w.ResponseWriter.WriteHeader(status)
}

func (w *gatewayResponseWriter) Write(p []byte) (int, error) {
	w.Header().Del(aibridge.HeaderGatewayError)
	return w.ResponseWriter.Write(p)
}

func (w *gatewayResponseWriter) Flush() {
	w.Header().Del(aibridge.HeaderGatewayError)
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}
