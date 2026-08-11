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
	return DupClientConfiguringTransport(client, header, nil)
}

// DupClientConfiguringTransport duplicates the Client like DupClientCopyingHeaders and, when configure is non-nil,
// calls it on the new transport before use. Callers that need to tune the connection pool go through this rather than
// reaching into the returned client, which would mean asserting a transport shape this function already has in hand.
func DupClientConfiguringTransport(client *codersdk.Client, header http.Header, configure func(*http.Transport)) (*codersdk.Client, error) {
	nc := codersdk.New(client.URL, codersdk.WithLogger(client.Logger()))
	nc.SessionTokenProvider = client.SessionTokenProvider
	newHeader, t, err := extractHeaderAndInnerTransport(client.HTTPClient.Transport)
	if err != nil {
		return nil, xerrors.Errorf("extract headers: %w", err)
	}
	maps.Copy(newHeader, header)

	transport := t.Clone()
	if configure != nil {
		configure(transport)
	}

	nc.HTTPClient.Transport = &codersdk.HeaderTransport{
		Transport: transport,
		Header:    newHeader,
	}
	return nc, nil
}

func extractHeaderAndInnerTransport(rt http.RoundTripper) (http.Header, *http.Transport, error) {
	if t, ok := rt.(*http.Transport); ok {
		// base case
		return make(http.Header), t, nil
	}
	if ht, ok := rt.(*codersdk.HeaderTransport); ok {
		headers, t, err := extractHeaderAndInnerTransport(ht.Transport)
		if err != nil {
			return nil, nil, err
		}
		maps.Copy(headers, ht.Header)
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
