//go:build !race

// This test is excluded from the race detector because the underlying
// hc-install library makes massive allocations and can take 1-2 minutes
// to complete.
package terraform_test

import (
	"archive/zip"
	"context"
	"net"
	"net/http"
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
	mainIndexJSONPrefix = `{
  "name": "terraform",
  "versions": {
`

	mainIndexJSONVersionTemplate = `
  "${ver}": {
    "builds": [
      {
        "arch": "amd64",
        "filename": "terraform_${ver}_linux_amd64.zip",
        "name": "terraform",
        "os": "linux",
        "url": "/terraform/${ver}/terraform_${ver}_linux_amd64.zip",
        "version": "${ver}"
      }
    ],
    "name": "terraform",
    "version": "${ver}"
  }`

	mainIndexJSONSufix = `  }
}
`

	versionedIndexJSONTemplate = `{
  "builds": [
    {
      "arch": "amd64",
      "filename": "terraform_${ver}_linux_amd64.zip",
      "name": "terraform",
      "os": "linux",
      "url": "/terraform/${ver}/terraform_${ver}_linux_amd64.zip",
      "version": "${ver}"
    }
  ],
  "name": "terraform",
  "version": "${ver}"
}
`
	terraformExecutableTemplate = `#!/bin/bash
cat <<EOF
{
  "terraform_version": "${ver}",
  "platform": "linux_amd64",
  "provider_selections": {},
  "terraform_outdated": true
}
EOF
`
	zipFilenameTemplate = "terraform_${ver}_linux_amd64.zip"
)

var (
	version1 = terraform.TerraformVersion
	version2 = version.Must(version.NewVersion("1.2.0"))
)

// Mock files are based on https://releases.hashicorp.com/terraform
// mock directory structure:
//
//	${tmpDir}/index.json
//	${tmpDir}/${version}/index.json
//	${tmpDir}/${version}/terraform_${version}_linux_amd64.zip
//	  -> zip contains 'terraform' binary and sometimes 'LICENSE.txt'
func createFakeTerraformInstallationFiles(t *testing.T) string {
	tmpDir := t.TempDir()
	mainV1 := strings.ReplaceAll(mainIndexJSONVersionTemplate, "${ver}", version1.String())
	mainV2 := strings.ReplaceAll(mainIndexJSONVersionTemplate, "${ver}", version2.String())
	mainIndex := mainIndexJSONPrefix + mainV1 + ",\n" + mainV2 + "\n" + mainIndexJSONSufix

	jsonV1 := strings.ReplaceAll(versionedIndexJSONTemplate, "${ver}", version1.String())
	jsonV2 := strings.ReplaceAll(versionedIndexJSONTemplate, "${ver}", version2.String())

	exe1Content := strings.ReplaceAll(terraformExecutableTemplate, "${ver}", version1.String())
	exe2Content := strings.ReplaceAll(terraformExecutableTemplate, "${ver}", version2.String())

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "index.json"), []byte(mainIndex), 0o400))
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, version1.String()), 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, version2.String()), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, version1.String(), "index.json"), []byte(jsonV1), 0o400))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, version2.String(), "index.json"), []byte(jsonV2), 0o400))

	zip1, err := os.Create(filepath.Join(tmpDir, version1.String(), strings.ReplaceAll(zipFilenameTemplate, "${ver}", version1.String())))
	require.NoError(t, err)
	zip2, err := os.Create(filepath.Join(tmpDir, version2.String(), strings.ReplaceAll(zipFilenameTemplate, "${ver}", version2.String())))
	require.NoError(t, err)
	zip1Writer := zip.NewWriter(zip1)
	zip2Writer := zip.NewWriter(zip2)

	exe1, err := zip1Writer.Create("terraform")
	require.NoError(t, err)
	bc, err := exe1.Write([]byte(exe1Content))
	require.NoError(t, err)
	require.NotZero(t, bc)

	lic1, err := zip1Writer.Create("LICENSE.txt")
	require.NoError(t, err)
	bc, err = lic1.Write([]byte("some license"))
	require.NoError(t, err)
	require.NotZero(t, bc)

	exe2, err := zip2Writer.Create("terraform")
	require.NoError(t, err)

	bc, err = exe2.Write([]byte(exe2Content))
	require.NoError(t, err)
	require.NotZero(t, bc)

	require.NoError(t, zip1Writer.Close())
	require.NoError(t, zip2Writer.Close())

	return tmpDir
}

// starts fake http server serving fake terraform installation files
func startFakeTerraformServer(t *testing.T, tmpDir string) (*http.Server, string) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener")
	}

	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir(tmpDir))
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

	tmpDir := createFakeTerraformInstallationFiles(t)
	srv, addr := startFakeTerraformServer(t, tmpDir)
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
