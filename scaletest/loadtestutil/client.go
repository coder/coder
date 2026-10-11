package loadtestutil

import (
	"context"
	"maps"
	"net/http"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// DupClientCopyingHeaders duplicates the client with an independent underlying
// HTTP transport while retaining its header providers. Additional headers take
// precedence over headers from the original client.
func DupClientCopyingHeaders(client *codersdk.Client, header http.Header) (*codersdk.Client, error) {
	nc := codersdk.New(client.URL, codersdk.WithLogger(client.Logger()))
	nc.SessionTokenProvider = client.SessionTokenProvider
	provider, t, err := extractHeaderAndInnerTransport(client.HTTPClient.Transport)
	if err != nil {
		return nil, xerrors.Errorf("extract headers: %w", err)
	}
	nc.HTTPClient.Transport = &codersdk.HeaderTransport{
		Transport: t.Clone(),
		Provider: nestedHeaderProvider{
			inner:  provider,
			static: header.Clone(),
		},
	}
	return nc, nil
}

// nestedHeaderProvider resolves current headers before adding its static headers.
type nestedHeaderProvider struct {
	inner  codersdk.HeaderProvider
	static http.Header
}

func (p nestedHeaderProvider) Headers(ctx context.Context) (http.Header, error) {
	headers, err := p.inner.Headers(ctx)
	if err != nil {
		return nil, err
	}
	headers = headers.Clone()
	if headers == nil {
		headers = make(http.Header)
	}
	maps.Copy(headers, p.static.Clone())
	return headers, nil
}

func extractHeaderAndInnerTransport(rt http.RoundTripper) (codersdk.HeaderProvider, *http.Transport, error) {
	var provider codersdk.HeaderProvider = codersdk.StaticHeaderProvider{}
	// Load-test clients have at most one header transport around the base transport.
	if ht, ok := rt.(*codersdk.HeaderTransport); ok {
		if ht.Provider != nil {
			provider = ht.Provider
		}
		rt = ht.Transport
	}
	// We assume only one layer of nesting for HeaderTransports.
	t, ok := rt.(*http.Transport)
	if !ok {
		// unrecognized RoundTripper. Just return a default transport, since we only care about preserving headers.
		t, ok = http.DefaultTransport.(*http.Transport)
		if !ok {
			return nil, nil, xerrors.New("DefaultTransport is not *http.Transport")
		}
	}
	return provider, t, nil
}
