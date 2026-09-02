package loadtestutil

import (
	"maps"
	"net/http"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// DupClientCopyingHeaders duplicates the Client, but with an independent underlying HTTP transport, so that it will not
// share connections with the client being duplicated. It copies any headers already on the existing transport as
// [codersdk.HeaderTransport] and add the headers in the argument.
func DupClientCopyingHeaders(client *codersdk.Client, header http.Header) (*codersdk.Client, error) {
	nc := codersdk.New(client.URL, codersdk.WithLogger(client.Logger()))
	nc.SessionTokenProvider = client.SessionTokenProvider
	headers, t, err := headersAndInnerTransport(client.HTTPClient.Transport)
	if err != nil {
		return nil, xerrors.Errorf("extract headers: %w", err)
	}
	nc.HTTPClient.Transport = &codersdk.HeaderTransport{
		Transport: t.Clone(),
		// Follows the source client's headers if they refresh over time.
		HeaderFunc: func() http.Header {
			h := headers().Clone()
			if h == nil {
				h = http.Header{}
			}
			maps.Copy(h, header)
			return h
		},
	}
	return nc, nil
}

// headersAndInnerTransport returns a getter for the headers rt adds and the
// *http.Transport at the bottom of the chain.
func headersAndInnerTransport(rt http.RoundTripper) (func() http.Header, *http.Transport, error) {
	headers := func() http.Header { return nil }
	if ht, ok := rt.(*codersdk.HeaderTransport); ok {
		headers = ht.Headers
		for ok {
			rt = ht.Transport
			ht, ok = rt.(*codersdk.HeaderTransport)
		}
	}
	if t, ok := rt.(*http.Transport); ok {
		return headers, t, nil
	}
	// unrecognized RoundTripper. Just return a default transport, since we only care about preserving headers.
	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		// unhittable, unless the Go stdlib changes.
		return nil, nil, xerrors.New("DefaultTransport is not *http.Transport")
	}
	return headers, t, nil
}
