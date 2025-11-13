//go:build !race

// This test is excluded from the race detector because the underlying
// hc-install library makes massive allocations and can take 1-2 minutes
// to complete.
package terraform_test

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
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
	// simple script that mocks `./terraform version -json`
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
)

var (
	version1 = terraform.TerraformVersion
	version2 = version.Must(version.NewVersion("1.2.0"))
)

type productBuild struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Filename string `json:"filename"`
	URL      string `json:"url"`
}

type productVersion struct {
	Name    string           `json:"name"`
	Version *version.Version `json:"version"`
	Builds  []productBuild   `json:"builds"`
}

type product struct {
	Name     string                    `json:"name"`
	Versions map[string]productVersion `json:"versions"`
}

func zipFilename(v *version.Version) string {
	return fmt.Sprintf("terraform_%s_linux_amd64.zip", v)
}

// returns `/${version}/index.json` in struct format
func versionedJSON(v *version.Version) productVersion {
	return productVersion{
		Name:    "terraform",
		Version: v,
		Builds: []productBuild{
			{
				Arch:     "amd64",
				Filename: zipFilename(v),
				Name:     "terraform",
				OS:       "linux",
				URL:      fmt.Sprintf("/terraform/%s/%s", v, zipFilename(v)),
				Version:  v.String(),
			},
		},
	}
}

// returns `/index.json` in struct format
func mainJSON(versions ...*version.Version) product {
	vj := map[string]productVersion{}
	for _, v := range versions {
		vj[v.String()] = versionedJSON(v)
	}
	mj := product{
		Name:     "terraform",
		Versions: vj,
	}
	return mj
}

func exeContent(v *version.Version) []byte {
	return []byte(strings.ReplaceAll(terraformExecutableTemplate, "${ver}", v.String()))
}

func mustMarshal(t *testing.T, obj any) []byte {
	b, err := json.Marshal(obj)
	require.NoError(t, err)
	return b
}

// Mock files are based on https://releases.hashicorp.com/terraform
// mock directory structure:
//
//	${tmpDir}/index.json
//	${tmpDir}/${version}/index.json
//	${tmpDir}/${version}/terraform_${version}_linux_amd64.zip
//	  -> zip contains 'terraform' binary and sometimes 'LICENSE.txt'
func createFakeTerraformInstallationFiles(t *testing.T) string {
	tmpDir := t.TempDir()

	mij := mustMarshal(t, mainJSON(version1, version2))
	jv1 := mustMarshal(t, versionedJSON(version1))
	jv2 := mustMarshal(t, versionedJSON(version2))

	// `index.json`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "index.json"), mij, 0o400))

	// `${version1}/index.json`
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, version1.String()), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, version1.String(), "index.json"), jv1, 0o400))

	// `${version2}/index.json`
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, version2.String()), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, version2.String(), "index.json"), jv2, 0o400))

	// `${version1}/linux_amd64.zip`
	zip1, err := os.Create(filepath.Join(tmpDir, version1.String(), zipFilename(version1)))
	require.NoError(t, err)
	zip1Writer := zip.NewWriter(zip1)

	// `${version1}/linux_amd64.zip/terraform`
	exe1, err := zip1Writer.Create("terraform")
	require.NoError(t, err)
	n, err := exe1.Write(exeContent(version1))
	require.NoError(t, err)
	require.NotZero(t, n)

	// `${version1}/linux_amd64.zip/LICENSE.txt`
	lic1, err := zip1Writer.Create("LICENSE.txt")
	require.NoError(t, err)
	n, err = lic1.Write([]byte("some license"))
	require.NoError(t, err)
	require.NotZero(t, n)
	require.NoError(t, zip1Writer.Close())

	// `${version2}/linux_amd64.zip`
	zip2, err := os.Create(filepath.Join(tmpDir, version2.String(), zipFilename(version2)))
	require.NoError(t, err)
	zip2Writer := zip.NewWriter(zip2)

	// `${version1}/linux_amd64.zip/terraform`
	exe2, err := zip2Writer.Create("terraform")
	require.NoError(t, err)
	n, err = exe2.Write(exeContent(version2))
	require.NoError(t, err)
	require.NotZero(t, n)
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
