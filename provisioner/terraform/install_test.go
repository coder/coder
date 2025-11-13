//go:build !race

// This test is excluded from the race detector because the underlying
// hc-install library makes massive allocations and can take 1-2 minutes
// to complete.
package terraform_test

import (
	"context"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisioner/terraform"
	"github.com/coder/coder/v2/testutil"
)

// starts fake http server serving fake terraform installation files
func startFakeTerraformServer(t *testing.T) (*http.Server, string) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener")
	}

	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("./testdata/fake-terraform-installer"))
	mux.Handle("/terraform/", http.StripPrefix("/terraform", fs))

	server := http.Server{
		ReadHeaderTimeout: time.Second,
		Handler:           mux,
	}
	go server.Serve(listener)
	return &server, "http://" + listener.Addr().String()
}

func TestInstall(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}
	ctx := context.Background()
	dir := t.TempDir()
	log := testutil.Logger(t)

	srv, addr := startFakeTerraformServer(t)
	defer func() {
		err := srv.Close()
		if err != nil {
			t.Errorf("failed to close server: %v", err)
		}
	}()

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
				p, err := terraform.Install(ctx, log, false, dir, version, addr, false)
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

	version1 := version.Must(version.NewVersion("1.13.4"))
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
	version2 := version.Must(version.NewVersion("1.2.0"))

	// Sanity-check
	require.NotEqual(t, version2.String(), version1.String())

	install(version2)

	modTime3 := checkBinModTime()
	require.Greater(t, modTime3, modTime2)
}
