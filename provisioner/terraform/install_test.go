//go:build !race

// This test is excluded from the race detector because the underlying
// hc-install library makes massive allocations and can take 1-2 minutes
// to complete.
package terraform_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisioner/terraform"
	"github.com/coder/coder/v2/testutil"
)

const (
	cacheSubDir  = "terraform_install_test"
	terraformURL = "https://releases.hashicorp.com"
)

var (
	version1 = terraform.TerraformVersion
	version2 = version.Must(version.NewVersion("1.2.0"))
)

type terraformProxy struct {
	t          *testing.T
	cacheRoot  string
	listener   net.Listener
	srv        *http.Server
	fsHandler  http.Handler
	httpClient *http.Client
	mutex      *sync.Mutex
}

// Simple cached proxy for terraform files.
// Serves files from persistent cache or forwards requests to releases.hashicorp.com
// Modifies downloaded index.json files so they point to proxy.
func persistentlyCachedProxy(t *testing.T) *terraformProxy {
	cacheRoot := filepath.Join(testutil.PersistentCacheDir(t), cacheSubDir)
	proxy := terraformProxy{
		t:          t,
		mutex:      &sync.Mutex{},
		cacheRoot:  cacheRoot,
		fsHandler:  http.FileServer(http.Dir(cacheRoot)),
		httpClient: &http.Client{},
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener")
	}
	proxy.listener = listener

	m := http.NewServeMux()
	m.HandleFunc("GET /", proxy.handleGet)

	proxy.srv = &http.Server{
		WriteTimeout: 30 * time.Second,
		ReadTimeout:  30 * time.Second,
		Handler:      m,
	}
	return &proxy
}

func uriToFilename(u url.URL) string {
	return strings.ReplaceAll(u.RequestURI(), "/", "_")
}

func (p *terraformProxy) handleGet(w http.ResponseWriter, r *http.Request) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	filename := uriToFilename(*r.URL)
	path := filepath.Join(p.cacheRoot, filename)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		require.NoError(p.t, os.MkdirAll(p.cacheRoot, os.ModeDir|0o700))

		// Update cache
		req, err := http.NewRequestWithContext(p.t.Context(), "GET", terraformURL+r.URL.Path, nil)
		require.NoError(p.t, err)

		resp, err := p.httpClient.Do(req)
		require.NoError(p.t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(p.t, err)

		// update index.json so urls in it point to proxy by making them relative
		// "https://releases.hashicorp.com/terraform/1.14.1/terraform_1.14.1_windows_amd64.zip" -> "/terraform/1.14.1/terraform_1.14.1_windows_amd64.zip"
		if strings.HasSuffix(r.URL.Path, "index.json") {
			body = []byte(strings.ReplaceAll(string(body), terraformURL, ""))
		}
		require.NoError(p.t, os.WriteFile(path, body, 0o400))
	} else if err != nil {
		p.t.Errorf("unexpected error when trying to read file from cache: %v", err)
	}

	// Serve from cache
	r.URL.Path = filename
	r.URL.RawPath = filename
	p.fsHandler.ServeHTTP(w, r)
}

func TestInstall(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}
	ctx := context.Background()
	dir := t.TempDir()
	log := testutil.Logger(t)

	proxy := persistentlyCachedProxy(t)
	go proxy.srv.Serve(proxy.listener)
	t.Cleanup(func() {
		require.NoError(t, proxy.srv.Close())
	})

	// Install spins off 8 installs with Version and waits for them all
	// to complete. The locking mechanism within Install should
	// prevent multiple binaries from being installed, so the function
	// should perform like a single install.
	install := func(version *version.Version) string {
		var wg sync.WaitGroup
		paths := make(chan string, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				p, err := terraform.Install(ctx, log, false, dir, version, "http://"+proxy.listener.Addr().String())
				assert.NoError(t, err)
				paths <- p
			}()
		}
		go func() {
			wg.Wait()
			close(paths)
		}()
		var firstPath string
		for p := range paths {
			if firstPath == "" {
				firstPath = p
			} else {
				require.Equal(t, firstPath, p, "installs returned different paths")
			}
		}
		return firstPath
	}

	binPath := install(version1)

	checkBinModTime := func() time.Time {
		binInfo, err := os.Stat(binPath)
		require.NoError(t, err)
		require.Greater(t, binInfo.Size(), int64(0))
		return binInfo.ModTime()
	}

	modTime1 := checkBinModTime()

	// Since we're using the same version the install should be idempotent.
	install(version1)
	modTime2 := checkBinModTime()
	require.Equal(t, modTime1, modTime2)

	// Ensure a new install happens when version changes
	// Sanity-check
	require.NotEqual(t, version2.String(), version1.String())

	install(version2)

	modTime3 := checkBinModTime()
	require.Greater(t, modTime3, modTime2)
}
