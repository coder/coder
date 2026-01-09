package loadtestutil

import (
	"net/http"

	"github.com/coder/coder/v2/codersdk"
)

// AddHeadersToClient adds the given headers, including any headers already on the existing transport as
// [codersdk.HeaderTransport].
func AddHeadersToClient(client *codersdk.Client, header http.Header) {
	if ht, ok := client.HTTPClient.Transport.(*codersdk.HeaderTransport); ok {
		for k, v := range header {
			ht.Header[k] = v
		}
	} else {
		client.HTTPClient.Transport = &codersdk.HeaderTransport{
			Transport: client.HTTPClient.Transport,
			Header:    ht.Header,
		}
	}
}
