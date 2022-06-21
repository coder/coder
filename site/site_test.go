package site_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

const (
	binCoderSha256  = "bin/coder.sha256"
	binCoderTarZstd = "bin/coder.tar.zst"
)

func sampleBinFS() fstest.MapFS {
	return fstest.MapFS{
		binCoderSha256: &fstest.MapFile{
			Data: []byte("9da308c2e4bc33afa72df5c088b5fc5673c477f3ef21d6bdaa358393834f9804 *coder-linux-amd64\n"),
		},
		binCoderTarZstd: &fstest.MapFile{
			// echo -n compressed >coder-linux-amd64
			// shasum -b -a 256 coder-linux-amd64 | tee coder.sha256
			// tar cf coder.tar coder-linux-amd64
			// zstd --long --ultra -22 coder.tar
			Data: []byte{
				0x28, 0xb5, 0x2f, 0xfd, 0x64, 0x00, 0x27, 0x4d, 0x05, 0x00, 0xf2, 0x49,
				0x1e, 0x1a, 0x90, 0xb7, 0x0d, 0x00, 0x8d, 0x0d, 0x68, 0x68, 0xea, 0x6b,
				0x77, 0x17, 0xec, 0xce, 0xb2, 0x97, 0x24, 0x25, 0x45, 0x2e, 0x4d, 0xbf,
				0xeb, 0x46, 0x94, 0x01, 0xa7, 0x56, 0x6b, 0x4b, 0x39, 0x20, 0x49, 0x3a,
				0xab, 0xda, 0xae, 0x89, 0x07, 0x60, 0x57, 0xc8, 0x17, 0x89, 0xc5, 0x40,
				0x28, 0x14, 0x01, 0xc3, 0x88, 0x0c, 0xc6, 0x8b, 0xd6, 0x58, 0xd4, 0xfb,
				0xaf, 0x0e, 0x57, 0xa9, 0x6c, 0x2d, 0x3b, 0x87, 0xd3, 0x1e, 0xef, 0xc4,
				0xfa, 0x40, 0x08, 0x53, 0x33, 0xa5, 0xe7, 0xd9, 0xef, 0xa6, 0x83, 0x97,
				0x26, 0x84, 0xfb, 0xca, 0x38, 0xf6, 0xc8, 0x19, 0xa3, 0x3c, 0xa2, 0x2d,
				0x87, 0xe6, 0x1c, 0x35, 0x13, 0xcf, 0x8f, 0x2b, 0x2f, 0xb7, 0x72, 0x1e,
				0x6b, 0x62, 0x9f, 0x64, 0x76, 0x3c, 0x6f, 0xfd, 0x41, 0x13, 0x28, 0x4b,
				0x25, 0x29, 0x11, 0x20, 0x50, 0x2b, 0x2e, 0x6d, 0x03, 0xf2, 0x21, 0xef,
				0xc7, 0xa8, 0x04, 0xc0, 0xb5, 0x0d, 0x1c, 0x20, 0x53, 0x08, 0xee, 0x0c,
				0xa0, 0x62, 0xa5, 0x42, 0x11, 0xc1, 0xa0, 0x49, 0x36, 0x5a, 0x21, 0x93,
				0x04, 0x69, 0x38, 0x3b, 0x20, 0xb3, 0x03, 0x5c, 0x34, 0xf5, 0x2f, 0x1f,
				0xeb, 0x39, 0xe8,
			},
		},
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

	sampleBinFSCorrupted := sampleBinFS()
	copy(sampleBinFSCorrupted[binCoderTarZstd].Data[10:], bytes.Repeat([]byte{0}, 10)) // Zero portion of archive.

	sampleBinFSMissingSha256 := sampleBinFS()
	delete(sampleBinFSMissingSha256, binCoderSha256)

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
			fs:   sampleBinFS(),
			reqs: []req{
				{url: "/bin/coder-linux-amd64", wantStatus: http.StatusOK, wantBody: []byte("compressed")},
				{url: "/bin/GITKEEP", wantStatus: http.StatusNotFound},
			},
		},
		{
			name:    "Extract and serve bin fails due to missing sha256",
			fs:      sampleBinFSMissingSha256,
			wantErr: true,
		},
		{
			name:    "Error on invalid archive",
			fs:      sampleBinFSCorrupted,
			wantErr: true,
		},
		{
			name: "Error on malformed sha256 file",
			fs: fstest.MapFS{
				binCoderSha256:  &fstest.MapFile{Data: []byte("byebye")},
				binCoderTarZstd: sampleBinFS()[binCoderTarZstd],
			},
			wantErr: true,
		},
		{
			name: "Error on empty archive",
			fs: fstest.MapFS{
				binCoderSha256:  &fstest.MapFile{Data: []byte{}},
				binCoderTarZstd: &fstest.MapFile{Data: []byte{}},
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

func TestExtractOrReadBinFS(t *testing.T) {
	t.Parallel()
	t.Run("DoubleExtractDoesNotModifyFiles", func(t *testing.T) {
		t.Parallel()

		siteFS := sampleBinFS()
		dest := t.TempDir()
		_, err := site.ExtractOrReadBinFS(dest, siteFS)
		require.NoError(t, err)

		checkModtime := func() map[string]time.Time {
			m := make(map[string]time.Time)

			err = filepath.WalkDir(dest, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				stat, err := d.Info()
				if err != nil {
					return err
				}

				m[path] = stat.ModTime()
				return nil
			})
			require.NoError(t, err)

			return m
		}

		firstModtimes := checkModtime()

		_, err = site.ExtractOrReadBinFS(dest, siteFS)
		require.NoError(t, err)

		secondModtimes := checkModtime()

		assert.Equal(t, firstModtimes, secondModtimes, "second extract should not modify files")
	})
	t.Run("SHA256MismatchCausesReExtract", func(t *testing.T) {
		t.Parallel()

		siteFS := sampleBinFS()
		dest := t.TempDir()
		_, err := site.ExtractOrReadBinFS(dest, siteFS)
		require.NoError(t, err)

		bin := filepath.Join(dest, "bin", "coder-linux-amd64")
		f, err := os.OpenFile(bin, os.O_WRONLY, 0o600)
		require.NoError(t, err)

		dontWant := []byte("hello")
		_, err = f.WriteAt(dontWant, 0) // Overwrite the start of file.
		assert.NoError(t, err)          // Assert to allow f.Close.

		err = f.Close()
		require.NoError(t, err)

		_, err = site.ExtractOrReadBinFS(dest, siteFS)
		require.NoError(t, err)

		f, err = os.Open(bin)
		require.NoError(t, err)
		defer f.Close()

		got := make([]byte, 5) // hello
		_, err = bufio.NewReader(f).Read(got)
		require.NoError(t, err)

		assert.NotEqual(t, dontWant, got, "file should be overwritten on hash mismatch")
	})
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
