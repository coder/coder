package aibridged

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridge"
)

// NewHTTPTransportFactory routes delegated inference to a standalone Gateway.
// target is the validated Gateway origin, optionally with a deployment prefix.
// The transport preserves streaming and cancellation and never retries requests.
func NewHTTPTransportFactory(target *url.URL, key string) aibridge.TransportFactory {
	base := *target
	// URL.JoinPath otherwise retains a relative Path for an empty base path,
	// which net/http cannot send as an origin-form request target.
	if base.Path == "" {
		base.Path = "/"
	}
	return &httpTransportFactory{target: base, key: key}
}

type httpTransportFactory struct {
	target url.URL
	key    string
}

func (f *httpTransportFactory) TransportFor(providerName string, source aibridge.Source) (http.RoundTripper, error) {
	if providerName == "" || providerName == "." || providerName == ".." || strings.ContainsAny(providerName, "/\\%?#") {
		return nil, xerrors.New("AI Gateway provider name must be a single path segment")
	}
	return &httpRoundTripper{factory: f, providerName: providerName, source: source}, nil
}

type httpRoundTripper struct {
	factory      *httpTransportFactory
	providerName string
	source       aibridge.Source
}

func (t *httpRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	id, ok := aibridge.DelegatedAPIKeyIDFromContext(req.Context())
	if !ok {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, xerrors.New("AI Gateway HTTP transport requires WithDelegatedAPIKeyID on the request context")
	}
	target := t.factory.target.JoinPath(aibridge.AIGatewayRootPath, url.PathEscape(t.providerName), req.URL.EscapedPath())
	target.RawQuery = req.URL.RawQuery
	cloned := req.Clone(req.Context())
	cloned.URL = target
	cloned.Host = target.Host
	cloned.Header.Set(aibridge.HeaderGatewayKey, t.factory.key)
	cloned.Header.Set(aibridge.HeaderDelegatedAPIKeyID, id)
	cloned.Header.Set(aibridge.HeaderDelegatedSource, string(t.source))
	cloned.Header.Del(aibridge.HeaderDelegatedWorkspace)
	cloned.Header.Del(aibridge.HeaderGatewayError)
	if attr, ok := aibridge.DelegatedAttributionFromContext(req.Context()); ok && attr.WorkspaceID != uuid.Nil {
		cloned.Header.Set(aibridge.HeaderDelegatedWorkspace, attr.WorkspaceID.String())
	}
	// Disable net/http's replay of requests marked idempotent. Inference must
	// only be retried by chatd's bounded generation retry policy.
	cloned.GetBody = nil
	resp, err := http.DefaultTransport.RoundTrip(cloned)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && resp.Header.Get(aibridge.HeaderGatewayError) == aibridge.GatewayKeyMismatchCode {
		_ = resp.Body.Close()
		return nil, aibridge.ErrGatewayKeyMismatch
	}
	// A redirect must not cause http.Client to replay inference or send the
	// delegated identity and shared key to a different destination.
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		_ = resp.Body.Close()
		return nil, xerrors.New("AI Gateway inference endpoint must not redirect requests")
	}
	return resp, nil
}
