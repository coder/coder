package utils

import (
	"io"
	"net/http"
)

// DropRequestTrailers removes all outbound request trailers. Outgoing trailers
// must be declared before RoundTrip starts, so clearing the cloned request map
// prevents values populated later on the inbound request from being sent.
func DropRequestTrailers(r *http.Request) {
	r.Trailer = nil
}

// DropResponseTrailers removes all upstream response trailers, including values
// that the transport populates only after the body reaches EOF.
func DropResponseTrailers(r *http.Response) {
	r.Trailer = nil
	if r.Body != nil {
		r.Body = &responseTrailerDroppingBody{ReadCloser: r.Body, response: r}
	}
}

type responseTrailerDroppingBody struct {
	io.ReadCloser
	response *http.Response
}

func (b *responseTrailerDroppingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.response.Trailer = nil
	return n, err
}

func (b *responseTrailerDroppingBody) Close() error {
	err := b.ReadCloser.Close()
	b.response.Trailer = nil
	return err
}
