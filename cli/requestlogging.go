package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type loggingRoundTripper struct {
	http.RoundTripper
	io.Writer

	logRequestBodies  bool
	logResponseBodies bool
}

func (lrt loggingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	inner := lrt.RoundTripper
	if inner == nil {
		inner = http.DefaultTransport
	}

	var requestBody bytes.Buffer
	if lrt.logRequestBodies {
		teeReader := io.TeeReader(request.Body, &requestBody)
		wrappedBody := &readCloser{teeReader, request.Body}
		request = request.Clone(request.Context())
		request.Body = wrappedBody
	}

	response, err := inner.RoundTrip(request)

	var displayedStatusCode string
	if err != nil {
		displayedStatusCode = "(err)"
	} else {
		displayedStatusCode = strconv.Itoa(response.StatusCode)
	}
	_, _ = fmt.Fprintf(lrt.Writer, "sending request: %s %s status: %s\n", request.Method, request.URL.String(), displayedStatusCode)

	if lrt.logRequestBodies {
		printRequestBodyLog(lrt.Writer, request, &requestBody)
	}

	if lrt.logResponseBodies {
		response = wrapResponse(lrt.Writer, response)
	}

	return response, err
}

type readCloser struct {
	io.Reader
	io.Closer
}

func formatBody(contentType string, body *bytes.Buffer) string {
	bareType, _, _ := strings.Cut(contentType, ";")
	if bareType == "application/json" || strings.HasPrefix(bareType, "text/") {
		return fmt.Sprintf("%s: %s", contentType, body.Bytes())
	}
	return fmt.Sprintf("%s, %d bytes", contentType, body.Len())
}

func printRequestBodyLog(writer io.Writer, request *http.Request, body *bytes.Buffer) {
	// omit bodies that are empty and expected to be empty
	if body.Len() == 0 && emptyRequestExpected(request.Method) {
		return
	}

	message := formatBody(request.Header.Get("Content-Type"), body)
	_, _ = fmt.Fprintf(writer, "\trequest body: %s\n", message)
}

func emptyRequestExpected(method string) bool {
	return !(method == "POST" || method == "PUT" || method == "PATCH")
}

type responseBodyLogger struct {
	writer io.Writer

	body        io.ReadCloser
	bodyContent bytes.Buffer
	contentType string
}

func wrapResponse(writer io.Writer, response *http.Response) *http.Response {
	newResponse := new(http.Response)
	*newResponse = *response

	logger := responseBodyLogger{
		writer:      writer,
		contentType: response.Header.Get("Content-Type"),
	}
	logger.body = &readCloser{io.TeeReader(response.Body, &logger.bodyContent), response.Body}
	newResponse.Body = &logger

	return newResponse
}

func (logger *responseBodyLogger) Read(p []byte) (int, error) {
	return logger.body.Read(p)
}

func (logger *responseBodyLogger) Close() error {
	message := formatBody(logger.contentType, &logger.bodyContent)
	_, _ = fmt.Fprintf(logger.writer, "\tresponse body: %s\n", message)
	return logger.body.Close()
}
