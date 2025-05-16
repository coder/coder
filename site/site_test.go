package site_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
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

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/site"
	"github.com/coder/coder/v2/testutil"
)

func TestInjection(t *testing.T) {
	t.Parallel()

	siteFS := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("{{ .User }}"),
		},
	}
	binFs := http.FS(fstest.MapFS{})
	db := dbmem.New()
	handler := site.New(&site.Options{
		Telemetry: telemetry.NewNoop(),
		BinFS:     binFs,
		Database:  db,
		SiteFS:    siteFS,
	})

	user := dbgen.User(t, db, database.User{})
	_, token := dbgen.APIKey(t, db, database.APIKey{
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set(codersdk.SessionTokenHeader, token)
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, r)
	require.Equal(t, http.StatusOK, rw.Code)
	var got codersdk.User
	err := json.Unmarshal([]byte(html.UnescapeString(rw.Body.String())), &got)
	require.NoError(t, err)

	// This will update as part of the request!
	got.LastSeenAt = user.LastSeenAt

	require.Equal(t, db2sdk.User(user, []uuid.UUID{}), got)
}

func TestInjectionFailureProducesCleanHTML(t *testing.T) {
	t.Parallel()

	db := dbmem.New()

	// Create an expired user with a refresh token, but provide no OAuth2
	// configuration so that refresh is impossible, this should result in
	// an error when httpmw.ExtractAPIKey is called.
	user := dbgen.User(t, db, database.User{})
	_, token := dbgen.APIKey(t, db, database.APIKey{
		UserID:    user.ID,
		LastUsed:  dbtime.Now().Add(-time.Hour),
		ExpiresAt: dbtime.Now().Add(-time.Second),
		LoginType: database.LoginTypeGithub,
	})
	_ = dbgen.UserLink(t, db, database.UserLink{
		UserID:            user.ID,
		LoginType:         database.LoginTypeGithub,
		OAuthRefreshToken: "hello",
		OAuthExpiry:       dbtime.Now().Add(-time.Second),
	})

	binFs := http.FS(fstest.MapFS{})
	siteFS := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("<html>{{ .User }}</html>"),
		},
	}
	handler := site.New(&site.Options{
		Telemetry: telemetry.NewNoop(),
		BinFS:     binFs,
		Database:  db,
		SiteFS:    siteFS,

		// No OAuth2 configs, refresh will fail.
		OAuth2Configs: &httpmw.OAuth2Configs{
			Github: nil,
			OIDC:   nil,
		},
	})

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set(codersdk.SessionTokenHeader, token)
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, r)

	// Ensure we get a clean HTML response with no user data or errors
	// from httpmw.ExtractAPIKey.
	assert.Equal(t, http.StatusOK, rw.Code)
	body := rw.Body.String()
	assert.Equal(t, "<html></html>", body)
}

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

	db, _ := dbtestutil.NewDB(t)
	srv := httptest.NewServer(site.New(&site.Options{
		Telemetry: telemetry.NewNoop(),
		BinFS:     binFS,
		SiteFS:    rootFS,
		Database:  db,
	}))
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
		"install.sh": &fstest.MapFile{
			Data: []byte("install-sh-bytes"),
		},
	}
	binFS := http.FS(fstest.MapFS{})

	db, _ := dbtestutil.NewDB(t)
	srv := httptest.NewServer(site.New(&site.Options{
		Telemetry: telemetry.NewNoop(),
		BinFS:     binFS,
		SiteFS:    rootFS,
		Database:  db,
	}))
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

		// Install script
		{"/install.sh", "install-sh-bytes"},
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
		url              string
		ifNoneMatch      string
		wantStatus       int
		wantBody         []byte
		wantOriginalSize int
		wantEtag         string
		compression      bool
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
					url:              "/bin/coder-linux-amd64",
					wantStatus:       http.StatusOK,
					wantBody:         []byte("compressed"),
					wantOriginalSize: 10,
					wantEtag:         fmt.Sprintf("%q", sampleBinSHAs["coder-linux-amd64"]),
				},
				// Test ETag support.
				{
					url:              "/bin/coder-linux-amd64",
					ifNoneMatch:      fmt.Sprintf("%q", sampleBinSHAs["coder-linux-amd64"]),
					wantStatus:       http.StatusNotModified,
					wantOriginalSize: 10,
					wantEtag:         fmt.Sprintf("%q", sampleBinSHAs["coder-linux-amd64"]),
				},
				// Test compression support with X-Original-Content-Length
				// header.
				{
					url:              "/bin/coder-linux-amd64",
					wantStatus:       http.StatusOK,
					wantOriginalSize: 10,
					compression:      true,
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
				{
					url:              "/bin/coder-linux-amd64",
					wantStatus:       http.StatusOK,
					wantBody:         []byte("embed"),
					wantOriginalSize: 5,
				},
				{
					url:              "/bin/coder_linux_amd64",
					wantStatus:       http.StatusOK,
					wantBody:         []byte("embed"),
					wantOriginalSize: 5,
				},
				{
					url:              "/bin/GITKEEP",
					wantStatus:       http.StatusOK,
					wantBody:         []byte(""),
					wantOriginalSize: 0,
				},
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

			site := site.New(&site.Options{
				Telemetry: telemetry.NewNoop(),
				BinFS:     binFS,
				BinHashes: binHashes,
				SiteFS:    rootFS,
			})
			compressor := middleware.NewCompressor(1, "text/*", "application/*")
			srv := httptest.NewServer(compressor.Handler(site))
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
					if tr.compression {
						req.Header.Set("Accept-Encoding", "gzip")
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

					if tr.compression {
						assert.Equal(t, "gzip", resp.Header.Get("Content-Encoding"), "content encoding is not gzip")
					} else {
						assert.Empty(t, resp.Header.Get("Content-Encoding"), "content encoding is not empty")
					}

					if tr.wantEtag != "" {
						assert.NotEmpty(t, resp.Header.Get("ETag"), "etag header is empty")
						assert.Equal(t, tr.wantEtag, resp.Header.Get("ETag"), "etag did not match")
					}

					if tr.wantOriginalSize > 0 {
						// This is a custom header that we set to help the
						// client know the size of the decompressed data. See
						// the comment in site.go.
						headerStr := resp.Header.Get("X-Original-Content-Length")
						assert.NotEmpty(t, headerStr, "X-Original-Content-Length header is empty")
						originalSize, err := strconv.Atoi(headerStr)
						if assert.NoErrorf(t, err, "could not parse X-Original-Content-Length header %q", headerStr) {
							assert.EqualValues(t, tr.wantOriginalSize, originalSize, "X-Original-Content-Length did not match")
						}
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

func TestRenderStaticErrorPageNoStatus(t *testing.T) {
	t.Parallel()

	d := site.ErrorPageData{
		HideStatus:   true,
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
	require.NotContains(t, bodyStr, strconv.Itoa(d.Status))
	require.Contains(t, bodyStr, d.Title)
	require.Contains(t, bodyStr, d.Description)
	require.Contains(t, bodyStr, "Retry")
	require.Contains(t, bodyStr, d.DashboardURL)
}

func TestJustFilesSystem(t *testing.T) {
	t.Parallel()

	tfs := fstest.MapFS{
		"dir/foo.txt": &fstest.MapFile{
			Data: []byte("hello world"),
		},
		"dir/bar.txt": &fstest.MapFile{
			Data: []byte("hello world"),
		},
	}

	mux := chi.NewRouter()
	mux.Mount("/onlyfiles/", http.StripPrefix("/onlyfiles", http.FileServer(http.FS(site.OnlyFiles(tfs)))))
	mux.Mount("/all/", http.StripPrefix("/all", http.FileServer(http.FS(tfs))))

	// The /all/ endpoint should serve the directory listing.
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, httptest.NewRequest("GET", "/all/dir/", nil))
	require.Equal(t, http.StatusOK, resp.Code, "all serves the directory")

	resp = httptest.NewRecorder()
	mux.ServeHTTP(resp, httptest.NewRequest("GET", "/onlyfiles/dir/", nil))
	require.Equal(t, http.StatusNotFound, resp.Code, "onlyfiles does not serve the directory")
}
