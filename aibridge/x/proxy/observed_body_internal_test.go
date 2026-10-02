package proxy

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestObservedBody(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		n    int
		err  error
	}{
		{name: "Bytes", n: 3},
		{name: "BytesAndEOF", n: 3, err: io.EOF},
		{name: "BytesAndError", n: 3, err: io.ErrUnexpectedEOF},
		{name: "EOF", err: io.EOF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			inner := &trackedBody{Reader: readerFunc(func(p []byte) (int, error) {
				return copy(p, "abc"[:tc.n]), tc.err
			})}
			var observed []byte
			body := &observedBody{ReadCloser: inner, onRead: func(p []byte) { observed = append(observed, p...) }}
			buf := make([]byte, 8)
			n, err := body.Read(buf)
			require.Equal(t, tc.n, n)
			require.ErrorIs(t, err, tc.err)
			require.Equal(t, "abc"[:tc.n], string(buf[:n]))
			require.Equal(t, "abc"[:tc.n], string(observed))
			require.Equal(t, errors.Is(tc.err, io.EOF), body.eof)
			if errors.Is(tc.err, io.EOF) {
				require.NoError(t, body.readErr)
			} else {
				require.ErrorIs(t, body.readErr, tc.err)
			}
			require.NoError(t, body.Close())
			require.True(t, body.closed)
			require.Equal(t, 1, inner.closes)
		})
	}
}
