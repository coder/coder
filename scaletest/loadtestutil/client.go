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
	getHeaders, t, err := extractHeaderAndInnerTransport(client.HTTPClient.Transport)
	if err != nil {
		return nil, xerrors.Errorf("extract headers: %w", err)
	}
	header = header.Clone()
	nc.HTTPClient.Transport = &codersdk.HeaderTransport{
		Transport: t.Clone(),
		Provider: dynamicHeaderProvider{getHeaders: func(ctx context.Context) (http.Header, error) {
			headers, err := getHeaders(ctx)
			if err != nil {
				return nil, err
			}
			maps.Copy(headers, header.Clone())
			return headers, nil
		}},
	}
	return nc, nil
}

type dynamicHeaderProvider struct {
	getHeaders func(context.Context) (http.Header, error)
}

func (p dynamicHeaderProvider) Headers(ctx context.Context) (http.Header, error) {
	return p.getHeaders(ctx)
}

func extractHeaderAndInnerTransport(rt http.RoundTripper) (func(context.Context) (http.Header, error), *http.Transport, error) {
	if t, ok := rt.(*http.Transport); ok {
		return func(context.Context) (http.Header, error) {
			return make(http.Header), nil
		}, t, nil
	}
	if ht, ok := rt.(*codersdk.HeaderTransport); ok {
		getHeaders, t, err := extractHeaderAndInnerTransport(ht.Transport)
		if err != nil {
			return nil, nil, err
		}
		return func(ctx context.Context) (http.Header, error) {
			headers, err := getHeaders(ctx)
			if err != nil {
				return nil, err
			}
			if ht.Provider != nil {
				provided, err := ht.Provider.Headers(ctx)
				if err != nil {
					return nil, err
				}
				maps.Copy(headers, provided.Clone())
			}
			return headers, nil
		}, t, nil
	}
	// Unrecognized RoundTripper: use a default transport and preserve headers.
	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, nil, xerrors.New("DefaultTransport is not *http.Transport")
	}
	return func(context.Context) (http.Header, error) {
		return make(http.Header), nil
	}, t, nil
}
