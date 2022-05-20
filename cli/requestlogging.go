package cli

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
)

type loggingRoundTripper struct {
	http.RoundTripper
	io.Writer
}

func newLoggingRoundTripper(writer io.Writer) http.RoundTripper {
	return &loggingRoundTripper{
		Writer: writer,
	}
}

func (lrt loggingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	inner := lrt.RoundTripper
	if inner == nil {
		inner = http.DefaultTransport
	}

	response, err := inner.RoundTrip(request)

	var displayedStatusCode string
	if err != nil {
		displayedStatusCode = "(err)"
	} else {
		displayedStatusCode = strconv.Itoa(response.StatusCode)
	}

	_, _ = fmt.Fprintf(lrt.Writer, "%s %s %s\n", request.Method, request.URL.String(), displayedStatusCode)
	return response, err
}
