//go:build !race

// This test is excluded from the race detector because the underlying
// hc-install library makes massive allocations and can take 1-2 minutes
// to complete.
package terraform_test

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	// simple scripts that mock `./terraform version -json`
	bashExecutableTemplate = `#!/bin/bash
cat <<EOF
{
  "terraform_version": "${ver}",
  "platform": "${os}_${arch}",
  "provider_selections": {},
  "terraform_outdated": true
}
EOF
`
	windowsExecutableGoSourceCodeTemplate = `package main

import "fmt"

func main() {
	fmt.Printf(` + "`" + `{
  "terraform_version": "%s",
  "platform": "windows_%s",
  "provider_selections": {},
  "terraform_outdated": true
}` + "`" + `)
}
`
)

var (
	version1 = terraform.TerraformVersion
	version2 = version.Must(version.NewVersion("1.2.0"))

	allPlatforms = []osArch{
		{"darwin", "amd64"},
		{"darwin", "arm64"},
		{"freebsd", "386"},
		{"freebsd", "amd64"},
		{"freebsd", "arm"},
		{"linux", "386"},
		{"linux", "amd64"},
		{"linux", "arm"},
		{"linux", "arm64"},
		{"openbsd", "386"},
		{"openbsd", "amd64"},
		{"solaris", "amd64"},
		{"windows", "386"},
		{"windows", "amd64"},
	}
)

type osArch struct {
	os   string
	arch string
}

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

func zipFilename(v *version.Version, osys string, arch string) string {
	return fmt.Sprintf("terraform_%s_%s_%s.zip", v, osys, arch)
}

func mockProductBuild(v *version.Version, osys string, arch string) productBuild {
	return productBuild{
		Arch:     arch,
		Filename: zipFilename(v, osys, arch),
		Name:     "terraform",
		OS:       osys,
		URL:      fmt.Sprintf("/terraform/%s/%s", v, zipFilename(v, osys, arch)),
		Version:  v.String(),
	}
}

// returns `/${version}/index.json` in struct format
func mockProductVersion(v *version.Version) productVersion {
	pv := productVersion{
		Name:    "terraform",
		Version: v,
	}
	for _, platform := range allPlatforms {
		pv.Builds = append(pv.Builds, mockProductBuild(v, platform.os, platform.arch))
	}
	return pv
}

// returns `/index.json` in struct format
func mockProduct(versions ...*version.Version) product {
	vj := map[string]productVersion{}
	for _, v := range versions {
		vj[v.String()] = mockProductVersion(v)
	}
	mj := product{
		Name:     "terraform",
		Versions: vj,
	}
	return mj
}

// for linux/mac simple script works
func unixExeContent(t *testing.T, v *version.Version, platform osArch) []byte {
	rep := strings.NewReplacer("${ver}", v.String(), "${os}", platform.os, "${arch}", platform.arch)
	return []byte(rep.Replace(bashExecutableTemplate))
}

// for windows it seems program compilation is required
func windowsExeContent(t *testing.T, tmpDir string, v *version.Version, platform osArch) []byte {
	code := fmt.Sprintf(windowsExecutableGoSourceCodeTemplate, v, platform.arch)
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "fake-terraform.go"), []byte(code), 0o600))

	var errbuf strings.Builder
	if _, err := os.Stat(filepath.Join(tmpDir, "go.mod")); errors.Is(err, os.ErrNotExist) {
		cmd := exec.Command("go", "mod", "init", "fake-terraform")
		cmd.Dir = tmpDir
		cmd.Stderr = &errbuf
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("failed to init go module: stdout: %s  stderr: %s  err: %v", output, errbuf.String(), err)
		}
	}

	exePath := filepath.Join(tmpDir, "terraform.exe")
	errbuf.Reset()
	cmd := exec.Command("go", "build", "-o", exePath)
	cmd.Dir = tmpDir
	cmd.Stderr = &errbuf
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to compile fake binary: stdout: %s  stderr: %s  err: %v", output, errbuf.String(), err)
	}
	exeContent, err := os.ReadFile(exePath)
	require.NoError(t, err)
	return exeContent
}

func mustCreateZips(t *testing.T, tmpDir string, v *version.Version) {
	for _, platform := range allPlatforms {
		if platform.os != runtime.GOOS || platform.arch != runtime.GOARCH {
			// only zip for platform on which test is being run is needed
			continue
		}

		// `${version}/${os}_${arch}.zip`
		zipFile, err := os.Create(filepath.Join(tmpDir, version1.String(), zipFilename(v, platform.os, platform.arch)))
		require.NoError(t, err)
		zipWriter := zip.NewWriter(zipFile)

		// `${version}/${os}_${arch}.zip/terraform{.exe}`
		var exe io.Writer

		if platform.os == "windows" {
			exe, err = zipWriter.Create("terraform.exe")
			require.NoError(t, err)
			n, err := exe.Write(windowsExeContent(t, tmpDir, v, platform))
			require.NoError(t, err)
			require.NotZero(t, n)
		} else {
			exe, err = zipWriter.Create("terraform")
			require.NoError(t, err)
			n, err := exe.Write(unixExeContent(t, v, platform))
			require.NoError(t, err)
			require.NotZero(t, n)
		}

		// not all versions include LICENSE files (eg. 1.2.0)
		if !v.Equal(version2) {
			// `${version}/${os}_${arch}.zip/LICENSE.txt`
			lic, err := zipWriter.Create("LICENSE.txt")
			require.NoError(t, err)
			n, err := lic.Write([]byte("some license"))
			require.NoError(t, err)
			require.NotZero(t, n)
		}
		require.NoError(t, zipWriter.Close())
	}
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

	// `index.json`
	mij := mustMarshal(t, mockProduct(version1, version2))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "index.json"), mij, 0o400))

	// `${version1}/index.json`
	jv1 := mustMarshal(t, mockProductVersion(version1))
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, version1.String()), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, version1.String(), "index.json"), jv1, 0o400))

	// `${version2}/index.json`
	jv2 := mustMarshal(t, mockProductVersion(version2))
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, version2.String()), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, version2.String(), "index.json"), jv2, 0o400))

	mustCreateZips(t, tmpDir, version1)
	mustCreateZips(t, tmpDir, version2)
	return tmpDir
}

// starts http server serving fake terraform installation files
func startFakeTerraformServer(t *testing.T, tmpDir string) string {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener")
	}

	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir(tmpDir))
	mux.Handle("/terraform/", http.StripPrefix("/terraform", fs))

	srv := http.Server{
		ReadHeaderTimeout: time.Second,
		Handler:           mux,
	}
	go srv.Serve(listener)
	t.Cleanup(func() {
		if err := srv.Close(); err != nil {
			t.Errorf("failed to close server: %v", err)
		}
	})
	return "http://" + listener.Addr().String()
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
	addr := startFakeTerraformServer(t, tmpDir)

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
