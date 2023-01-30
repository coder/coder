package site_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/site"
	"github.com/coder/coder/testutil"
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

	srv := httptest.NewServer(site.Handler(rootFS, binFS, nil))
	defer srv.Close()

	// Create a context
	ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitShort)
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

	srv := httptest.NewServer(site.Handler(rootFS, binFS, nil))
	defer srv.Close()

	// Create a context
	ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitShort)
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
	binCoderSha1    = "bin/coder.sha1"
	binCoderTarZstd = "bin/coder.tar.zst"
)

var sampleBinSHAs = map[string]string{
	"coder-linux-amd64": "55641d5d56bbb8ccf5850fe923bd971b86364604",
}

func sampleBinFS() fstest.MapFS {
	sha1File := bytes.NewBuffer(nil)
	for name, sha := range sampleBinSHAs {
		_, _ = fmt.Fprintf(sha1File, "%s *%s\n", sha, name)
	}
	return fstest.MapFS{
		binCoderSha1: &fstest.MapFile{
			Data: sha1File.Bytes(),
		},
		binCoderTarZstd: &fstest.MapFile{
			// echo -n compressed >coder-linux-amd64
			// shasum -b -a 1 coder-linux-amd64 | tee coder.sha1
			// tar cf coder.tar coder.sha1 coder-linux-amd64
			// zstd --long --ultra -22 coder.tar
			Data: []byte{
				0x28, 0xb5, 0x2f, 0xfd, 0x64, 0x00, 0x27, 0xb5, 0x04, 0x00, 0x12, 0x08,
				0x1a, 0x1a, 0x90, 0xa7, 0x0e, 0x00, 0x0c, 0x19, 0x7c, 0xfb, 0xa0, 0xa1,
				0x5d, 0x21, 0xee, 0xae, 0xa8, 0x35, 0x65, 0x26, 0x57, 0x6e, 0x9a, 0xee,
				0xaf, 0x77, 0x94, 0x01, 0xf8, 0xec, 0x3d, 0x86, 0x1c, 0xdc, 0xb1, 0x76,
				0x8d, 0x31, 0x8a, 0x00, 0xf6, 0x77, 0xa9, 0x48, 0x24, 0x06, 0x42, 0xa1,
				0x08, 0x14, 0x4e, 0x67, 0x5f, 0x47, 0x4a, 0x8f, 0xf1, 0x6a, 0x8d, 0xc1,
				0x5a, 0x36, 0xea, 0xb6, 0x16, 0x52, 0x4a, 0x79, 0x7f, 0xbf, 0xb2, 0x77,
				0x63, 0x4b, 0x0e, 0x4b, 0x41, 0x12, 0xe2, 0x25, 0x98, 0x05, 0x73, 0x53,
				0x35, 0x71, 0xf5, 0x68, 0x37, 0xb7, 0x61, 0x45, 0x3e, 0xd9, 0x47, 0x99,
				0x3d, 0x51, 0xd3, 0xe0, 0x09, 0x10, 0xf6, 0xc7, 0x0a, 0x10, 0x20, 0x50,
				0x2b, 0x2e, 0x6d, 0x03, 0xf2, 0x21, 0xef, 0xc7, 0xa8, 0xc4, 0x3b, 0x8c,
				0x03, 0x64, 0x1a, 0xd9, 0x9d, 0x01, 0x60, 0xac, 0x94, 0x5a, 0x08, 0x05,
				0x4d, 0xb2, 0xd1, 0x0a, 0x99, 0x14, 0x48, 0xe3, 0xd9, 0x01, 0x99, 0x1d,
				0xe0, 0xda, 0xd4, 0xbd, 0xd4, 0xc6, 0x51, 0x0d,
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
	delete(sampleBinFSMissingSha256, binCoderSha1)

	type req struct {
		url         string
		ifNoneMatch string
		wantStatus  int
		wantBody    []byte
		wantEtag    string
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
				{
					url:        "/bin/coder-linux-amd64",
					wantStatus: http.StatusOK,
					wantBody:   []byte("compressed"),
					wantEtag:   fmt.Sprintf("%q", sampleBinSHAs["coder-linux-amd64"]),
				},
				// Test ETag support.
				{
					url:         "/bin/coder-linux-amd64",
					ifNoneMatch: fmt.Sprintf("%q", sampleBinSHAs["coder-linux-amd64"]),
					wantStatus:  http.StatusNotModified,
					wantEtag:    fmt.Sprintf("%q", sampleBinSHAs["coder-linux-amd64"]),
				},
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
				binCoderSha1:    &fstest.MapFile{Data: []byte("byebye")},
				binCoderTarZstd: sampleBinFS()[binCoderTarZstd],
			},
			wantErr: true,
		},
		{
			name: "Error on empty archive",
			fs: fstest.MapFS{
				binCoderSha1:    &fstest.MapFile{Data: []byte{}},
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
			name: "Serve local fs when embed fs empty",
			fs:   fstest.MapFS{},
			reqs: []req{
				{url: "/bin/coder-linux-amd64", wantStatus: http.StatusNotFound},
				{url: "/bin/GITKEEP", wantStatus: http.StatusNotFound},
			},
		},
		{
			name: "Serve embed fs",
			fs: fstest.MapFS{
				"bin/GITKEEP": &fstest.MapFile{
					Data: []byte(""),
				},
				"bin/coder-linux-amd64": &fstest.MapFile{
					Data: []byte("embed"),
				},
			},
			reqs: []req{
				// We support both hyphens and underscores for compatibility.
				{url: "/bin/coder-linux-amd64", wantStatus: http.StatusOK, wantBody: []byte("embed")},
				{url: "/bin/coder_linux_amd64", wantStatus: http.StatusOK, wantBody: []byte("embed")},
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
			binFS, binHashes, err := site.ExtractOrReadBinFS(dest, tt.fs)
			if !tt.wantErr && err != nil {
				require.NoError(t, err, "extract or read failed")
			} else if tt.wantErr {
				require.Error(t, err, "extraction or read did not fail")
			}

			srv := httptest.NewServer(site.Handler(rootFS, binFS, binHashes))
			defer srv.Close()

			// Create a context
			ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitShort)
			defer cancelFunc()

			for _, tr := range tt.reqs {
				t.Run(strings.TrimPrefix(tr.url, "/"), func(t *testing.T) {
					req, err := http.NewRequestWithContext(ctx, "GET", srv.URL+tr.url, nil)
					require.NoError(t, err, "http request failed")

					if tr.ifNoneMatch != "" {
						req.Header.Set("If-None-Match", tr.ifNoneMatch)
					}

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
					if tr.wantStatus == http.StatusNoContent || tr.wantStatus == http.StatusNotModified {
						assert.Empty(t, gotBody, "body is not empty")
					}

					if tr.wantEtag != "" {
						assert.NotEmpty(t, resp.Header.Get("ETag"), "etag header is empty")
						assert.Equal(t, tr.wantEtag, resp.Header.Get("ETag"), "etag did not match")
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
		_, _, err := site.ExtractOrReadBinFS(dest, siteFS)
		require.NoError(t, err)

		checkModtime := func() map[string]time.Time {
			m := make(map[string]time.Time)

			err = filepath.WalkDir(dest, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil // Only check the files.
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

		_, _, err = site.ExtractOrReadBinFS(dest, siteFS)
		require.NoError(t, err)

		secondModtimes := checkModtime()

		assert.Equal(t, firstModtimes, secondModtimes, "second extract should not modify files")
	})
	t.Run("SHA256MismatchCausesReExtract", func(t *testing.T) {
		t.Parallel()

		siteFS := sampleBinFS()
		dest := t.TempDir()
		_, _, err := site.ExtractOrReadBinFS(dest, siteFS)
		require.NoError(t, err)

		bin := filepath.Join(dest, "bin", "coder-linux-amd64")
		f, err := os.OpenFile(bin, os.O_WRONLY, 0o600)
		require.NoError(t, err)

		dontWant := []byte("hello")
		_, err = f.WriteAt(dontWant, 0) // Overwrite the start of file.
		assert.NoError(t, err)          // Assert to allow f.Close.

		err = f.Close()
		require.NoError(t, err)

		_, _, err = site.ExtractOrReadBinFS(dest, siteFS)
		require.NoError(t, err)

		f, err = os.Open(bin)
		require.NoError(t, err)
		defer f.Close()

		got := make([]byte, 5) // hello
		_, err = f.Read(got)
		require.NoError(t, err)

		assert.NotEqual(t, dontWant, got, "file should be overwritten on hash mismatch")
	})

	t.Run("ParsesHashes", func(t *testing.T) {
		t.Parallel()

		siteFS := sampleBinFS()
		dest := t.TempDir()
		_, hashes, err := site.ExtractOrReadBinFS(dest, siteFS)
		require.NoError(t, err)

		require.Equal(t, sampleBinSHAs, hashes, "hashes did not match")
	})
}

func TestRenderStaticErrorPage(t *testing.T) {
	t.Parallel()

	d := site.ErrorPageData{
		Status:       http.StatusBadGateway,
		Title:        "Bad Gateway 1234",
		Description:  "shout out colin",
		RetryEnabled: true,
		DashboardURL: "https://example.com",
	}

	rw := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	site.RenderStaticErrorPage(rw, r, d)

	resp := rw.Result()
	defer resp.Body.Close()
	require.Equal(t, d.Status, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "text/html")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	bodyStr := string(body)
	require.Contains(t, bodyStr, strconv.Itoa(d.Status))
	require.Contains(t, bodyStr, d.Title)
	require.Contains(t, bodyStr, d.Description)
	require.Contains(t, bodyStr, "Retry")
	require.Contains(t, bodyStr, d.DashboardURL)
}
