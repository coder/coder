package cachecompress

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestCompressorEncodings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		path              string
		expectedEncoding  string
		acceptedEncodings []string
	}{
		{
			name:              "no expected encodings due to no accepted encodings",
			path:              "/file.html",
			acceptedEncodings: nil,
			expectedEncoding:  "",
		},
		{
			name:              "gzip is only encoding",
			path:              "/file.html",
			acceptedEncodings: []string{"gzip"},
			expectedEncoding:  "gzip",
		},
		{
			name:              "gzip is preferred over deflate",
			path:              "/file.html",
			acceptedEncodings: []string{"gzip", "deflate"},
			expectedEncoding:  "gzip",
		},
		{
			name:              "deflate is used",
			path:              "/file.html",
			acceptedEncodings: []string{"deflate"},
			expectedEncoding:  "deflate",
		},
		{
			name:              "nop is preferred",
			path:              "/file.html",
			acceptedEncodings: []string{"nop, gzip, deflate"},
			expectedEncoding:  "nop",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			logger := testutil.Logger(t)
			tempDir := t.TempDir()
			cacheDir := filepath.Join(tempDir, "cache")
			err := os.MkdirAll(cacheDir, 0o700)
			require.NoError(t, err)
			srcDir := filepath.Join(tempDir, "src")
			err = os.MkdirAll(srcDir, 0o700)
			require.NoError(t, err)
			err = os.WriteFile(filepath.Join(srcDir, "file.html"), []byte("textstring"), 0o600)
			require.NoError(t, err)

			compressor := NewCompressor(logger, prometheus.NewRegistry(), 5, cacheDir, http.FS(os.DirFS(srcDir)))
			if len(compressor.encoders) != 0 || len(compressor.pooledEncoders) != 2 {
				t.Errorf("gzip and deflate should be pooled")
			}
			logger.Debug(context.Background(), "started compressor")

			compressor.SetEncoder("nop", func(w io.Writer, _ int) io.WriteCloser {
				return nopEncoder{w}
			})

			if len(compressor.encoders) != 1 {
				t.Errorf("nop encoder should be stored in the encoders map")
			}

			ts := httptest.NewServer(compressor)
			defer ts.Close()
			// ctx := testutil.Context(t, testutil.WaitShort)
			ctx := context.Background()
			header, respString := testRequestWithAcceptedEncodings(ctx, t, ts, "GET", tc.path, tc.acceptedEncodings...)
			if respString != "textstring" {
				t.Errorf("response text doesn't match; expected:%q, got:%q", "textstring", respString)
			}
			if got := header.Get("Content-Encoding"); got != tc.expectedEncoding {
				t.Errorf("expected encoding %q but got %q", tc.expectedEncoding, got)
			}
		})
	}
}

func testRequestWithAcceptedEncodings(ctx context.Context, t *testing.T, ts *httptest.Server, method, path string, encodings ...string) (http.Header, string) {
	req, err := http.NewRequestWithContext(ctx, method, ts.URL+path, nil)
	if err != nil {
		t.Fatal(err)
		return nil, ""
	}
	if len(encodings) > 0 {
		encodingsString := strings.Join(encodings, ",")
		req.Header.Set("Accept-Encoding", encodingsString)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true // prevent automatically setting gzip

	resp, err := (&http.Client{Transport: transport}).Do(req)
	require.NoError(t, err)

	respBody := decodeResponseBody(t, resp)
	defer resp.Body.Close()

	return resp.Header, respBody
}

func decodeResponseBody(t *testing.T, resp *http.Response) string {
	var reader io.ReadCloser
	t.Logf("encoding: '%s'", resp.Header.Get("Content-Encoding"))
	rawBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	t.Logf("raw body: %x", rawBody)
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		var err error
		reader, err = gzip.NewReader(bytes.NewReader(rawBody))
		require.NoError(t, err)
	case "deflate":
		reader = flate.NewReader(bytes.NewReader(rawBody))
	default:
		return string(rawBody)
	}
	respBody, err := io.ReadAll(reader)
	require.NoError(t, err, "failed to read response body: %T %+v", err, err)
	err = reader.Close()
	require.NoError(t, err)

	return string(respBody)
}

type nopEncoder struct {
	io.Writer
}

func (nopEncoder) Close() error { return nil }

// nolint: tparallel // we want to assert the state of the cache, so run synchronously
func TestCompressorHeadings(t *testing.T) {
	t.Parallel()
	logger := testutil.Logger(t)
	tempDir := t.TempDir()
	cacheDir := filepath.Join(tempDir, "cache")
	err := os.MkdirAll(cacheDir, 0o700)
	require.NoError(t, err)
	srcDir := filepath.Join(tempDir, "src")
	err = os.MkdirAll(srcDir, 0o700)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(srcDir, "file.html"), []byte("textstring"), 0o600)
	require.NoError(t, err)

	compressor := NewCompressor(logger, prometheus.NewRegistry(), 5, cacheDir, http.FS(os.DirFS(srcDir)))

	ts := httptest.NewServer(compressor)
	defer ts.Close()

	tests := []struct {
		name string
		path string
	}{
		{
			name: "exists",
			path: "/file.html",
		},
		{
			name: "not found",
			path: "/missing.html",
		},
		{
			name: "not found directory",
			path: "/a_directory/",
		},
	}

	// nolint: paralleltest // we want to assert the state of the cache, so run synchronously
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := testutil.Context(t, testutil.WaitShort)
			req := httptest.NewRequestWithContext(ctx, "GET", tc.path, nil)

			// request directly from http.FileServer as our baseline response
			respROrig := httptest.NewRecorder()
			http.FileServer(http.Dir(srcDir)).ServeHTTP(respROrig, req)
			respOrig := respROrig.Result()

			req.Header.Add("Accept-Encoding", "gzip")
			// serve twice so that we go thru cache hit and cache miss code
			for range 2 {
				respRec := httptest.NewRecorder()
				compressor.ServeHTTP(respRec, req)
				respComp := respRec.Result()

				require.Equal(t, respOrig.StatusCode, respComp.StatusCode)
				for key, values := range respOrig.Header {
					if key == "Content-Length" {
						continue // we don't get length on compressed responses
					}
					require.Equal(t, values, respComp.Header[key])
				}
			}
		})
	}
	// only the cache hit should leave a file around
	files, err := os.ReadDir(srcDir)
	require.NoError(t, err)
	require.Len(t, files, 1)
}

func TestCompressor_SingleFlight(t *testing.T) {
	t.Parallel()
	logger := testutil.Logger(t)
	tempDir := t.TempDir()
	cacheDir := filepath.Join(tempDir, "cache")
	err := os.MkdirAll(cacheDir, 0o700)
	require.NoError(t, err)
	srcDir := filepath.Join(tempDir, "src")
	err = os.MkdirAll(srcDir, 0o700)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(srcDir, "file.html"), []byte("textstring"), 0o600)
	require.NoError(t, err)

	reg := prometheus.NewRegistry()
	compressor := NewCompressor(logger, reg, 5, cacheDir, http.FS(os.DirFS(srcDir)))

	ts := httptest.NewServer(compressor)
	defer ts.Close()

	ctx := testutil.Context(t, testutil.WaitShort)

	// Make 10 requests for the same file.
	for range 10 {
		req, err := http.NewRequestWithContext(ctx, "GET", ts.URL+"/file.html", nil)
		require.NoError(t, err)
		req.Header.Set("Accept-Encoding", "gzip")

		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.DisableCompression = true
		resp, err := (&http.Client{Transport: transport}).Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}

	// We should have 10 total requests: 1 miss + 9 hits.
	requestsHit := promtest.ToFloat64(compressor.metrics.requestsTotal.WithLabelValues("gzip", "true"))
	requestsMiss := promtest.ToFloat64(compressor.metrics.requestsTotal.WithLabelValues("gzip", "false"))
	require.Equal(t, float64(9), requestsHit, "expected 9 cache hits")
	require.Equal(t, float64(1), requestsMiss, "expected 1 cache miss")

	// We should have only 1 compression operation.
	compressions := promtest.ToFloat64(compressor.metrics.compressionsTotal.WithLabelValues("gzip"))
	require.Equal(t, float64(1), compressions, "expected only 1 compression")
}
