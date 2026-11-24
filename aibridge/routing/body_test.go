package routing_test

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/routing"
)

func encodeZstd(t *testing.T, src []byte) []byte {
	t.Helper()
	var dst bytes.Buffer
	zw, err := zstd.NewWriter(&dst)
	require.NoError(t, err)
	_, err = zw.Write(src)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return dst.Bytes()
}

func encodeGzip(t *testing.T, src []byte) []byte {
	t.Helper()
	var dst bytes.Buffer
	zw := gzip.NewWriter(&dst)
	_, err := zw.Write(src)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return dst.Bytes()
}

func encodeBrotli(t *testing.T, src []byte) []byte {
	t.Helper()
	var dst bytes.Buffer
	zw := brotli.NewWriter(&dst)
	_, err := zw.Write(src)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return dst.Bytes()
}

// encodeDeflate uses zlib because HTTP "deflate" is zlib-wrapped data
// (RFC 9110), not raw DEFLATE.
func encodeDeflate(t *testing.T, src []byte) []byte {
	t.Helper()
	var dst bytes.Buffer
	zw := zlib.NewWriter(&dst)
	_, err := zw.Write(src)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return dst.Bytes()
}

// Status mapping, the decoded size cap and corrupt data are covered at the
// bridge boundary by TestCompressedRequestBodyRejected.
func TestDecodeRequestBody(t *testing.T) {
	t.Parallel()

	plain := []byte("hello, encoded world")

	tests := []struct {
		name string
		// encodings holds one value per Content-Encoding header line.
		encodings []string
		body      []byte
		wantErr   error
	}{
		{name: "no header", body: plain},
		{name: "zstd", encodings: []string{"zstd"}, body: encodeZstd(t, plain)},
		{name: "gzip", encodings: []string{"gzip"}, body: encodeGzip(t, plain)},
		{name: "br", encodings: []string{"br"}, body: encodeBrotli(t, plain)},
		{name: "deflate", encodings: []string{"deflate"}, body: encodeDeflate(t, plain)},
		{name: "uppercase zstd", encodings: []string{"ZSTD"}, body: encodeZstd(t, plain)},
		{name: "compress unsupported", encodings: []string{"compress"}, body: plain, wantErr: routing.ErrUnsupportedContentEncoding},
		{name: "comma separated unsupported", encodings: []string{"gzip, zstd"}, body: plain, wantErr: routing.ErrUnsupportedContentEncoding},
		{name: "repeated header lines unsupported", encodings: []string{"gzip", "zstd"}, body: encodeZstd(t, encodeGzip(t, plain)), wantErr: routing.ErrUnsupportedContentEncoding},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(tt.body))
			req.Header.Set("X-Test", "unchanged")
			if len(tt.encodings) > 0 {
				for _, enc := range tt.encodings {
					req.Header.Add("Content-Encoding", enc)
				}
				req.Header.Set("Content-Length", "1234")
				req.ContentLength = 1234
			}
			headerBefore := req.Header.Clone()

			err := routing.DecodeRequestBody(req)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)

			got, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.Equal(t, plain, got)

			if len(tt.encodings) == 0 {
				require.Equal(t, headerBefore, req.Header)
				return
			}
			require.Empty(t, req.Header.Values("Content-Encoding"))
			require.Empty(t, req.Header.Get("Content-Length"))
			require.Equal(t, "unchanged", req.Header.Get("X-Test"))
			require.Equal(t, int64(len(plain)), req.ContentLength)
		})
	}
}
