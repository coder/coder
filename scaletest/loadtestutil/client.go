package loadtestutil

import (
	"context"
	"maps"
	"net/http"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// DupClientCopyingHeaders duplicates the Client, but with an independent underlying HTTP transport, so that it will not
// share connections with the client being duplicated. It copies any headers already on the existing transport as
// [codersdk.HeaderTransport] and add the headers in the argument.
func DupClientCopyingHeaders(ctx context.Context, client *codersdk.Client, header http.Header) (*codersdk.Client, error) {
	nc := codersdk.New(client.URL, codersdk.WithLogger(client.Logger()))
	nc.SessionTokenProvider = client.SessionTokenProvider
	newHeader, t, err := extractHeaderAndInnerTransport(ctx, client.HTTPClient.Transport)
	if err != nil {
		return nil, xerrors.Errorf("extract headers: %w", err)
	}
	maps.Copy(newHeader, header)

	nc.HTTPClient.Transport = &codersdk.HeaderTransport{
		Transport: t.Clone(),
		Provider:  codersdk.StaticHeaderProvider{Header: newHeader},
	}
	return nc, nil
}

func extractHeaderAndInnerTransport(ctx context.Context, rt http.RoundTripper) (http.Header, *http.Transport, error) {
	if t, ok := rt.(*http.Transport); ok {
		// base case
		return make(http.Header), t, nil
	}
	if ht, ok := rt.(*codersdk.HeaderTransport); ok {
		headers, t, err := extractHeaderAndInnerTransport(ctx, ht.Transport)
		if err != nil {
			return nil, nil, err
		}
		if ht.Provider != nil {
			provided, err := ht.Provider.Headers(ctx)
			if err != nil {
				return nil, nil, xerrors.Errorf("get headers: %w", err)
			}
			maps.Copy(headers, provided)
		}
		return headers, t, nil
	}
	// unrecognized RoundTripper. Just return a default transport, since we only care about preserving headers.
	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		// unhittable, unless the Go stdlib changes.
		return nil, nil, xerrors.New("DefaultTransport is not *http.Transport")
	}
	return make(http.Header), t, nil
}
