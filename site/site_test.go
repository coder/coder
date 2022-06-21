package site_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/site"
)

func TestCaching(t *testing.T) {
	t.Parallel()

	// Create a test server
	rootFS := fstest.MapFS{
		"bundle.js":        &fstest.MapFile{},
		"image.png":        &fstest.MapFile{},
		"static/image.png": &fstest.MapFile{},
		"favicon.ico": &fstest.MapFile{
			Data: []byte("folderFile"),
		},

		"service-worker.js": &fstest.MapFile{},
		"index.html": &fstest.MapFile{
			Data: []byte("folderFile"),
		},
		"terminal.html": &fstest.MapFile{
			Data: []byte("folderFile"),
		},
	}
	binFS := http.FS(fstest.MapFS{})

	srv := httptest.NewServer(site.Handler(rootFS, binFS))
	defer srv.Close()

	// Create a context
	ctx, cancelFunc := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelFunc()

	testCases := []struct {
		path             string
		isExpectingCache bool
	}{
		{"/bundle.js", true},
		{"/image.png", true},
		{"/static/image.png", true},
		{"/favicon.ico", true},

		{"/", false},
		{"/service-worker.js", false},
		{"/index.html", false},
		{"/double/nested/terminal.html", false},
	}

	for _, testCase := range testCases {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+testCase.path, nil)
		require.NoError(t, err, "create request")

		res, err := srv.Client().Do(req)
		require.NoError(t, err, "get index")

		cache := res.Header.Get("Cache-Control")
		if testCase.isExpectingCache {
			require.Equalf(t, "public, max-age=31536000, immutable", cache, "expected %w file to have immutable cache", testCase.path)
		} else {
			require.Equalf(t, "", cache, "expected %w file to not have immutable cache header", testCase.path)
		}

		require.NoError(t, res.Body.Close(), "closing response")
	}
}

func TestServingFiles(t *testing.T) {
	t.Parallel()

	// Create a test server
	rootFS := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("index-bytes"),
		},
		"favicon.ico": &fstest.MapFile{
			Data: []byte("favicon-bytes"),
		},
		"dashboard.js": &fstest.MapFile{
			Data: []byte("dashboard-js-bytes"),
		},
		"dashboard.css": &fstest.MapFile{
			Data: []byte("dashboard-css-bytes"),
		},
	}
	binFS := http.FS(fstest.MapFS{})

	srv := httptest.NewServer(site.Handler(rootFS, binFS))
	defer srv.Close()

	// Create a context
	ctx, cancelFunc := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelFunc()

	testCases := []struct {
		path     string
		expected string
	}{
		// Index cases
		{"/", "index-bytes"},
		{"/index.html", "index-bytes"},
		{"/nested", "index-bytes"},
		{"/nested/", "index-bytes"},
		{"/nested/index.html", "index-bytes"},

		// These are nested paths that should lead back to index. We don't
		// allow nested JS or CSS files.
		{"/double/nested", "index-bytes"},
		{"/double/nested/", "index-bytes"},
		{"/double/nested/index.html", "index-bytes"},
		{"/nested/dashboard.js", "index-bytes"},
		{"/nested/dashboard.css", "index-bytes"},
		{"/double/nested/dashboard.js", "index-bytes"},
		{"/double/nested/dashboard.css", "index-bytes"},

		// Favicon cases
		// The favicon is always root-referenced in index.html:
		{"/favicon.ico", "favicon-bytes"},

		// JS, CSS cases
		{"/dashboard.js", "dashboard-js-bytes"},
		{"/dashboard.css", "dashboard-css-bytes"},
	}

	for _, testCase := range testCases {
		path := srv.URL + testCase.path

		req, err := http.NewRequestWithContext(ctx, "GET", path, nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err, "get file")
		data, _ := io.ReadAll(resp.Body)
		require.Equal(t, string(data), testCase.expected, "Verify file: "+testCase.path)
		err = resp.Body.Close()
		require.NoError(t, err)
	}
}

func TestShouldCacheFile(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		reqFile  string
		expected bool
	}{
		{"123456789.js", true},
		{"apps/app/code/terminal.css", true},
		{"image.png", true},
		{"static/image.png", true},
		{"static/images/section-a/image.jpeg", true},

		{"service-worker.js", false},
		{"dashboard.html", false},
		{"apps/app/code/terminal.html", false},
	}

	for _, testCase := range testCases {
		got := site.ShouldCacheFile(testCase.reqFile)
		require.Equal(t, testCase.expected, got, fmt.Sprintf("Expected ShouldCacheFile(%s) to be %t", testCase.reqFile, testCase.expected))
	}
}

func TestServingBin(t *testing.T) {
	t.Parallel()

	// Create a misc rootfs for realistic test.
	rootFS := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("index-bytes"),
		},
		"favicon.ico": &fstest.MapFile{
			Data: []byte("favicon-bytes"),
		},
		"dashboard.js": &fstest.MapFile{
			Data: []byte("dashboard-js-bytes"),
		},
		"dashboard.css": &fstest.MapFile{
			Data: []byte("dashboard-css-bytes"),
		},
	}

	type req struct {
		url        string
		wantStatus int
		wantBody   []byte
	}
	tests := []struct {
		name    string
		fs      fstest.MapFS
		reqs    []req
		wantErr bool
	}{
		{
			name: "Extract and serve bin",
			fs: fstest.MapFS{
				"bin/coder.tar.zst": &fstest.MapFile{
					// echo iamcoder >coder-linux-amd64
					// tar cf coder.tar coder-linux-amd64
					// zstd --long --ultra -22 coder.tar
					Data: []byte{
						0x28, 0xb5, 0x2f, 0xfd, 0x64, 0x00, 0x27, 0xf5, 0x02, 0x00, 0x12, 0xc4,
						0x0e, 0x16, 0xa0, 0xb5, 0x39, 0x00, 0xe8, 0x67, 0x59, 0xaf, 0xe3, 0xdd,
						0x8d, 0xfe, 0x47, 0xe8, 0x9d, 0x9c, 0x44, 0x0b, 0x75, 0x70, 0x61, 0x52,
						0x0d, 0x56, 0xaa, 0x16, 0xb9, 0x5a, 0x0a, 0x4b, 0x40, 0xd2, 0x7a, 0x05,
						0xd1, 0xd7, 0xe3, 0xf9, 0xf9, 0x07, 0xef, 0xda, 0x77, 0x04, 0xff, 0xe8,
						0x7a, 0x94, 0x56, 0x9a, 0x40, 0x3b, 0x94, 0x61, 0x18, 0x91, 0x90, 0x21,
						0x0c, 0x00, 0xf3, 0xc5, 0xe5, 0xd8, 0x80, 0x10, 0x06, 0x0a, 0x08, 0x86,
						0xb2, 0x00, 0x60, 0x12, 0x70, 0xd3, 0x51, 0x05, 0x04, 0x20, 0x16, 0x2c,
						0x79, 0xad, 0x01, 0xc0, 0xf5, 0x28, 0x08, 0x03, 0x1c, 0x4c, 0x84, 0xf4,
					},
				},
			},
			reqs: []req{
				{url: "/bin/coder-linux-amd64", wantStatus: http.StatusOK, wantBody: []byte("iamcoder\n")},
				{url: "/bin/GITKEEP", wantStatus: http.StatusNotFound},
			},
		},
		{
			name: "Error on invalid archive",
			fs: fstest.MapFS{
				"bin/coder.tar.zst": &fstest.MapFile{
					Data: []byte{
						0x28, 0xb5, 0x2f, 0xfd, 0x64, 0x00, 0x27, 0xf5, 0x02, 0x00, 0x12, 0xc4,
						0x0e, 0x16, 0xa0, 0xb5, 0x39, 0x00, 0xe8, 0x67, 0x59, 0xaf, 0xe3, 0xdd,
						0x8d, 0xfe, 0x47, 0xe8, 0x9d, 0x9c, 0x44, 0x0b, 0x75, 0x70, 0x61, 0x52,
						0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Zeroed from above test.
						0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Zeroed from above test.
						0x7a, 0x94, 0x56, 0x9a, 0x40, 0x3b, 0x94, 0x61, 0x18, 0x91, 0x90, 0x21,
						0x0c, 0x00, 0xf3, 0xc5, 0xe5, 0xd8, 0x80, 0x10, 0x06, 0x0a, 0x08, 0x86,
						0xb2, 0x00, 0x60, 0x12, 0x70, 0xd3, 0x51, 0x05, 0x04, 0x20, 0x16, 0x2c,
						0x79, 0xad, 0x01, 0xc0, 0xf5, 0x28, 0x08, 0x03, 0x1c, 0x4c, 0x84, 0xf4,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Error on empty archive",
			fs: fstest.MapFS{
				"bin/coder.tar.zst": &fstest.MapFile{Data: []byte{}},
			},
			wantErr: true,
		},
		{
			name: "Serve local fs",
			fs: fstest.MapFS{
				// Only GITKEEP file on embedded fs, won't be served.
				"bin/GITKEEP": &fstest.MapFile{},
			},
			reqs: []req{
				{url: "/bin/coder-linux-amd64", wantStatus: http.StatusNotFound},
				{url: "/bin/GITKEEP", wantStatus: http.StatusNotFound},
			},
		},
		{
			name: "Serve local fs when embedd fs empty",
			fs:   fstest.MapFS{},
			reqs: []req{
				{url: "/bin/coder-linux-amd64", wantStatus: http.StatusNotFound},
				{url: "/bin/GITKEEP", wantStatus: http.StatusNotFound},
			},
		},
		{
			name: "Serve embedd fs",
			fs: fstest.MapFS{
				"bin/GITKEEP": &fstest.MapFile{
					Data: []byte(""),
				},
				"bin/coder-linux-amd64": &fstest.MapFile{
					Data: []byte("embedd"),
				},
			},
			reqs: []req{
				{url: "/bin/coder-linux-amd64", wantStatus: http.StatusOK, wantBody: []byte("embedd")},
				{url: "/bin/GITKEEP", wantStatus: http.StatusOK, wantBody: []byte("")},
			},
		},
	}
	//nolint // Parallel test detection issue.
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dest := t.TempDir()
			binFS, err := site.ExtractOrReadBinFS(dest, tt.fs)
			if !tt.wantErr && err != nil {
				require.NoError(t, err, "extract or read failed")
			} else if tt.wantErr {
				require.Error(t, err, "extraction or read did not fail")
			}

			srv := httptest.NewServer(site.Handler(rootFS, binFS))
			defer srv.Close()

			// Create a context
			ctx, cancelFunc := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancelFunc()

			for _, tr := range tt.reqs {
				t.Run(strings.TrimPrefix(tr.url, "/"), func(t *testing.T) {
					req, err := http.NewRequestWithContext(ctx, "GET", srv.URL+tr.url, nil)
					require.NoError(t, err, "http request failed")

					resp, err := http.DefaultClient.Do(req)
					require.NoError(t, err, "http do failed")
					defer resp.Body.Close()

					gotStatus := resp.StatusCode
					gotBody, _ := io.ReadAll(resp.Body)

					if tr.wantStatus > 0 {
						assert.Equal(t, tr.wantStatus, gotStatus, "status did not match")
					}
					if tr.wantBody != nil {
						assert.Equal(t, string(tr.wantBody), string(gotBody), "body did not match")
					}
				})
			}
		})
	}
}

func TestServeAPIResponse(t *testing.T) {
	t.Parallel()

	// Create a test server
	rootFS := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte(`{"code":{{ .APIResponse.StatusCode }},"message":"{{ .APIResponse.Message }}"}`),
		},
	}
	binFS := http.FS(fstest.MapFS{})

	apiResponse := site.APIResponse{
		StatusCode: http.StatusBadGateway,
		Message:    "This could be an error message!",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(site.WithAPIResponse(r.Context(), apiResponse))
		site.Handler(rootFS, binFS).ServeHTTP(w, r)
	}))
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), "GET", srv.URL, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	t.Logf("resp: %q", data)
	err = json.Unmarshal(data, &body)
	require.NoError(t, err)
	require.Equal(t, apiResponse.StatusCode, body.Code)
	require.Equal(t, apiResponse.Message, body.Message)
}
