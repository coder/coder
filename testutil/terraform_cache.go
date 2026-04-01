//go:build linux || darwin

package testutil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	// terraformConfigFileName is the name of the Terraform CLI config file.
	terraformConfigFileName = "terraform.rc"
	// cacheProvidersDirName is the subdirectory name for the provider mirror.
	cacheProvidersDirName = "providers"
	// cacheTemplateFilesDirName is the subdirectory name for template files.
	cacheTemplateFilesDirName = "files"
)

// hashTemplateFilesAndTestName generates a unique hash based on test name and template files.
func hashTemplateFilesAndTestName(t *testing.T, testName string, templateFiles map[string]string) string {
	t.Helper()

	sortedFileNames := make([]string, 0, len(templateFiles))
	for fileName := range templateFiles {
		sortedFileNames = append(sortedFileNames, fileName)
	}
	sort.Strings(sortedFileNames)

	// Inserting a delimiter between the file name and the file content
	// ensures that a file named `ab` with content `cd`
	// will not hash to the same value as a file named `abc` with content `d`.
	// This can still happen if the file name or content include the delimiter,
	// but hopefully they won't.
	delimiter := []byte("🎉 🌱 🌷")

	hasher := sha256.New()
	for _, fileName := range sortedFileNames {
		file := templateFiles[fileName]
		_, err := hasher.Write([]byte(fileName))
		require.NoError(t, err)
		_, err = hasher.Write(delimiter)
		require.NoError(t, err)
		_, err = hasher.Write([]byte(file))
		require.NoError(t, err)
	}
	_, err := hasher.Write(delimiter)
	require.NoError(t, err)
	_, err = hasher.Write([]byte(testName))
	require.NoError(t, err)

	return hex.EncodeToString(hasher.Sum(nil))
}

// WriteTFCliConfig writes a Terraform CLI config file (`terraform.rc`) in `dir` to enforce using the local provider mirror.
// This blocks network access for providers, forcing Terraform to use only what's cached in `dir`.
// Returns the path to the generated config file.
func WriteTFCliConfig(t *testing.T, dir string) string {
	t.Helper()

	cliConfigPath := filepath.Join(dir, terraformConfigFileName)
	require.NoError(t, os.MkdirAll(filepath.Dir(cliConfigPath), 0o700))

	content := fmt.Sprintf(`
		provider_installation {
			filesystem_mirror {
				path    = "%s"
				include = ["*/*"]
			}
			direct {
				exclude = ["*/*"]
			}
		}
	`, filepath.Join(dir, cacheProvidersDirName))
	require.NoError(t, os.WriteFile(cliConfigPath, []byte(content), 0o600))
	return cliConfigPath
}

func runCmd(t *testing.T, dir string, args ...string) {
	t.Helper()

	stdout, stderr := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	cmd := exec.Command(args[0], args[1:]...) //#nosec
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to run %s: %s\nstdout: %s\nstderr: %s", strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
}

// GetTestTFCacheDir returns a unique cache directory path based on the test name and template files.
// Each test gets a unique cache dir based on its name and template files.
// This ensures that tests can download providers in parallel and that they
// will redownload providers if the template files change.
func GetTestTFCacheDir(t *testing.T, rootDir string, testName string, templateFiles map[string]string) string {
	t.Helper()

	hash := hashTemplateFilesAndTestName(t, testName, templateFiles)
	dir := filepath.Join(rootDir, hash[:12])
	return dir
}

// DownloadTFProviders ensures Terraform providers are downloaded and cached locally in a unique directory for the test.
// Uses `terraform init` then `mirror` to populate the cache if needed.
// Returns the cache directory path.
func DownloadTFProviders(t *testing.T, rootDir string, testName string, templateFiles map[string]string) string {
	t.Helper()

	dir := GetTestTFCacheDir(t, rootDir, testName, templateFiles)
	if _, err := os.Stat(dir); err == nil {
		t.Logf("%s: using cached terraform providers", testName)
		return dir
	}
	filesDir := filepath.Join(dir, cacheTemplateFilesDirName)
	defer func() {
		// The files dir will contain a copy of terraform providers generated
		// by the terraform init command. We don't want to persist them since
		// we already have a registry mirror in the providers dir.
		if err := os.RemoveAll(filesDir); err != nil {
			t.Logf("failed to remove files dir %s: %s", filesDir, err)
		}
		if !t.Failed() {
			return
		}
		// If `DownloadTFProviders` function failed, clean up the cache dir.
		// We don't want to leave it around because it may be incomplete or corrupted.
		if err := os.RemoveAll(dir); err != nil {
			t.Logf("failed to remove dir %s: %s", dir, err)
		}
	}()

	require.NoError(t, os.MkdirAll(filesDir, 0o700))

	for fileName, file := range templateFiles {
		filePath := filepath.Join(filesDir, fileName)
		require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0o700))
		require.NoError(t, os.WriteFile(filePath, []byte(file), 0o600))
	}

	providersDir := filepath.Join(dir, cacheProvidersDirName)
	require.NoError(t, os.MkdirAll(providersDir, 0o700))

	// We need to run init because if a test uses modules in its template,
	// the mirror command will fail without it.
	runCmd(t, filesDir, "terraform", "init")
	// Now, mirror the providers into `providersDir`. We use this explicit mirror
	// instead of relying only on the standard Terraform plugin cache.
	//
	// Why? Because this mirror, when used with the CLI config from `WriteCliConfig`,
	// prevents Terraform from hitting the network registry during `plan`. This cuts
	// down on network calls, making CI tests less flaky.
	//
	// In contrast, the standard cache *still* contacts the registry for metadata
	// during `init`, even if the plugins are already cached locally - see link below.
	//
	// Ref: https://developer.hashicorp.com/terraform/cli/config/config-file#provider-plugin-cache
	// > When a plugin cache directory is enabled, the terraform init command will
	// > still use the configured or implied installation methods to obtain metadata
	// > about which plugins are available
	runCmd(t, filesDir, "terraform", "providers", "mirror", providersDir)

	return dir
}

// CacheTFProviders caches providers locally and generates a Terraform CLI config to use *only* that cache.
// This setup prevents network access for providers during `terraform init`, improving reliability
// in subsequent test runs.
// Returns the path to the generated CLI config file.
func CacheTFProviders(t *testing.T, rootDir string, testName string, templateFiles map[string]string) string {
	t.Helper()

	providersParentDir := DownloadTFProviders(t, rootDir, testName, templateFiles)
	cliConfigPath := WriteTFCliConfig(t, providersParentDir)
	return cliConfigPath
}
