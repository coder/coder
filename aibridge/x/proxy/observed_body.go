package proxy

import (
	"context"
	"io"
	"net/http"
)

type observationContextKey struct{}

type responseObservation struct {
	client         http.ResponseWriter
	status         int
	credentialHint string
	err            error
	body           *observedBody
}

func observationFromContext(ctx context.Context) *responseObservation {
	//nolint:forcetypeassert // The caller installs it before invoking the shared proxy.
	return ctx.Value(observationContextKey{}).(*responseObservation)
}

type observedBody struct {
	io.ReadCloser
	// onRead receives borrowed bytes and must copy anything it retains. It must
	// not mutate the bytes, block, or perform I/O.
	onRead  func([]byte)
	eof     bool
	closed  bool
	readErr error
}

func (b *observedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 && b.onRead != nil {
		b.onRead(p[:n])
	}
	if err == io.EOF {
		b.eof = true
	} else if b.readErr == nil {
		b.readErr = err
	}
	return n, err
}

func (b *observedBody) Close() error {
	b.closed = true
	return b.ReadCloser.Close()
}
