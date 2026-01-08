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
	newHeader, t, err := extractHeaderAndInnerTransport(client.HTTPClient.Transport)
	if err != nil {
		return nil, xerrors.Errorf("extract headers: %w", err)
	}
	maps.Copy(newHeader, header)

	nc.HTTPClient.Transport = &codersdk.HeaderTransport{
		Transport: t.Clone(),
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
	return nil, nil, xerrors.New("round tripper is neither HeaderTransport nor Transport")
}
