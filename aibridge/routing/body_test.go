package routing_test

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"

	"github.com/coder/coder/v2/aibridge/routing"
)

func encodeZstd(t *testing.T, src []byte) []byte {
	t.Helper()
	var dst bytes.Buffer
	zw, err := zstd.NewWriter(&dst)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := zw.Write(src); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return dst.Bytes()
}

func encodeGzip(t *testing.T, src []byte) []byte {
	t.Helper()
	var dst bytes.Buffer
	zw := gzip.NewWriter(&dst)
	if _, err := zw.Write(src); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return dst.Bytes()
}

func encodeBrotli(t *testing.T, src []byte) []byte {
	t.Helper()
	var dst bytes.Buffer
	zw := brotli.NewWriter(&dst)
	if _, err := zw.Write(src); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return dst.Bytes()
}

func encodeDeflate(t *testing.T, src []byte) []byte {
	t.Helper()
	var dst bytes.Buffer
	zw := zlib.NewWriter(&dst)
	if _, err := zw.Write(src); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return dst.Bytes()
}

func TestDecodeBody(t *testing.T) {
	t.Parallel()

	plain := []byte("hello, encoded world")

	tests := []struct {
		name     string
		encoding string
		body     []byte
		want     []byte
		wantErr  error
	}{
		{name: "no header", body: plain, want: plain},
		{name: "zstd", encoding: "zstd", body: encodeZstd(t, plain), want: plain},
		{name: "gzip", encoding: "gzip", body: encodeGzip(t, plain), want: plain},
		{name: "br", encoding: "br", body: encodeBrotli(t, plain), want: plain},
		{name: "deflate", encoding: "deflate", body: encodeDeflate(t, plain), want: plain},
		{name: "uppercase zstd", encoding: "ZSTD", body: encodeZstd(t, plain), want: plain},
		{name: "compress unsupported", encoding: "compress", body: plain, wantErr: routing.ErrUnsupportedContentEncoding},
		{name: "gzip zstd unsupported", encoding: "gzip, zstd", body: plain, wantErr: routing.ErrUnsupportedContentEncoding},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(tt.body))
			if tt.encoding != "" {
				req.Header.Set("Content-Encoding", tt.encoding)
				req.Header.Set("Content-Length", "1234")
				req.ContentLength = 1234
			} else {
				req.Header.Set("X-Test", "unchanged")
			}

			headerBefore := req.Header.Clone()
			err := routing.DecodeRequestBody(req)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("got error %v, want errors.Is(_, %v)", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("DecodeRequestBody() error = %v", err)
			}

			got, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("decoded body = %q, want %q", got, tt.want)
			}

			if tt.encoding == "" {
				if !headerEqual(req.Header, headerBefore) {
					t.Fatalf("headers changed: got %v, want %v", req.Header, headerBefore)
				}
				return
			}
			if got := req.Header.Get("Content-Encoding"); got != "" {
				t.Errorf("Content-Encoding header = %q, want gone", got)
			}
			if got := req.Header.Get("Content-Length"); got != "" {
				t.Errorf("Content-Length header = %q, want gone", got)
			}
			if req.ContentLength != int64(len(tt.want)) {
				t.Errorf("ContentLength = %d, want %d", req.ContentLength, len(tt.want))
			}
		})
	}
}

func TestDecodeBodyBomb(t *testing.T) {
	t.Parallel()

	bomb := bytes.Repeat([]byte("A"), 33<<20)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(encodeGzip(t, bomb)))
	req.Header.Set("Content-Encoding", "gzip")

	err := routing.DecodeRequestBody(req)
	if _, ok := errors.AsType[*http.MaxBytesError](err); !ok {
		t.Fatalf("got error %T (%v), want *http.MaxBytesError", err, err)
	}
}

func TestDecodeBodyGarbageZstd(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("definitely not zstd")))
	req.Header.Set("Content-Encoding", "zstd")

	err := routing.DecodeRequestBody(req)
	if err == nil {
		t.Fatal("DecodeRequestBody() error = nil, want zstd decode error")
	}
	if errors.Is(err, routing.ErrUnsupportedContentEncoding) {
		t.Fatalf("error = %v, want a zstd decode error rather than unsupported encoding", err)
	}
}

func headerEqual(a, b http.Header) bool {
	if len(a) != len(b) {
		return false
	}
	for key, values := range a {
		if !equalStrings(values, b[key]) {
			return false
		}
	}
	return true
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
