package proxy

import "io"

type observedBody struct {
	io.ReadCloser
	onRead  func([]byte)
	eof     bool
	readErr error
}

func newObservedBody(body io.ReadCloser, onRead func([]byte)) *observedBody {
	return &observedBody{ReadCloser: body, onRead: onRead}
}

func (b *observedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 && b.onRead != nil {
		b.onRead(p[:n])
	}
	if err == io.EOF {
		b.eof = true
	} else if err != nil && b.readErr == nil {
		b.readErr = err
	}
	return n, err
}
