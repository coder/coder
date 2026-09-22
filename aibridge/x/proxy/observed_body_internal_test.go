package proxy

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

var (
	errObservedRead  = xerrors.New("observed read failure")
	errObservedClose = xerrors.New("observed close failure")
)

type scriptedBody struct {
	data     []byte
	n        int
	err      error
	closeErr error
	closed   bool
}

func (b *scriptedBody) Read(p []byte) (int, error) {
	copy(p, b.data[:b.n])
	return b.n, b.err
}

func (b *scriptedBody) Close() error {
	b.closed = true
	return b.closeErr
}

func TestObservedBody(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		n       int
		err     error
		wantEOF bool
		wantErr error
	}{
		{name: "bytes without error", n: 3},
		{name: "bytes with EOF", n: 3, err: io.EOF, wantEOF: true},
		{name: "bytes with failure", n: 3, err: errObservedRead, wantErr: errObservedRead},
		{name: "EOF", err: io.EOF, wantEOF: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			wrapped := &scriptedBody{data: []byte("abc"), n: tc.n, err: tc.err, closeErr: errObservedClose}
			var observed []byte
			body := newObservedBody(wrapped, func(p []byte) { observed = append(observed, p...) })
			buf := make([]byte, 8)

			n, err := body.Read(buf)
			require.Equal(t, tc.n, n)
			require.ErrorIs(t, err, tc.err)
			if tc.n == 0 {
				require.Empty(t, observed)
			} else {
				require.Equal(t, []byte("abc")[:tc.n], observed)
			}
			require.Equal(t, tc.wantEOF, body.eof)
			require.ErrorIs(t, body.readErr, tc.wantErr)

			require.ErrorIs(t, body.Close(), errObservedClose)
			require.True(t, wrapped.closed)
		})
	}
}
