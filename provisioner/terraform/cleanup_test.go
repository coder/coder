//go:build linux || darwin

package terraform_test

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/provisioner/terraform"
	"github.com/coder/coder/v2/testutil"
)

const cachePath = "/tmp/coder/provisioner-0/tf"

// updateGoldenFiles is a flag that can be set to update golden files.
var updateGoldenFiles = flag.Bool("update", false, "Update golden files")

var (
	now              = time.Date(2023, 6, 3, 4, 5, 6, 0, time.UTC)
	coderPluginPath  = filepath.Join("registry.terraform.io", "coder", "coder", "0.11.1", "darwin_arm64")
	dockerPluginPath = filepath.Join("registry.terraform.io", "kreuzwerker", "docker", "2.25.0", "darwin_arm64")
)

func TestPluginCache_Golden(t *testing.T) {
	t.Parallel()

	prepare := func() (afero.Fs, slog.Logger) {
		// afero.MemMapFs does not modify atimes, so use a real FS instead.
		tmpDir := t.TempDir()
		fs := afero.NewBasePathFs(afero.NewOsFs(), tmpDir)
		logger := testutil.Logger(t).
			Leveled(slog.LevelDebug).
			Named("cleanup-test")
		return fs, logger
	}

	t.Run("all plugins are stale", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		fs, logger := prepare()

		// given
		// This plugin is older than 30 days.
		addPluginFile(t, fs, coderPluginPath, "terraform-provider-coder_v0.11.1", now.Add(-63*24*time.Hour))
		addPluginFile(t, fs, coderPluginPath, "LICENSE", now.Add(-33*24*time.Hour))
		addPluginFile(t, fs, coderPluginPath, "README.md", now.Add(-31*24*time.Hour))
		addPluginFolder(t, fs, coderPluginPath, "new_folder", now.Add(-31*24*time.Hour))
		addPluginFile(t, fs, coderPluginPath, filepath.Join("new_folder", "foobar.tf"), now.Add(-43*24*time.Hour))

		// This plugin is older than 30 days.
		addPluginFile(t, fs, dockerPluginPath, "terraform-provider-docker_v2.25.0", now.Add(-31*24*time.Hour))
		addPluginFile(t, fs, dockerPluginPath, "LICENSE", now.Add(-32*24*time.Hour))
		addPluginFile(t, fs, dockerPluginPath, "README.md", now.Add(-33*24*time.Hour))

		// when
		terraform.CleanStaleTerraformPlugins(ctx, cachePath, fs, now, logger)

		// then
		diffFileSystem(t, fs)
	})

	t.Run("one plugin is stale", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		fs, logger := prepare()

		// given
		addPluginFile(t, fs, coderPluginPath, "terraform-provider-coder_v0.11.1", now.Add(-2*time.Hour))
		addPluginFile(t, fs, coderPluginPath, "LICENSE", now.Add(-3*time.Hour))
		addPluginFile(t, fs, coderPluginPath, "README.md", now.Add(-4*time.Hour))
		addPluginFolder(t, fs, coderPluginPath, "new_folder", now.Add(-5*time.Hour))
		addPluginFile(t, fs, coderPluginPath, filepath.Join("new_folder", "foobar.tf"), now.Add(-4*time.Hour))

		// This plugin is older than 30 days.
		addPluginFile(t, fs, dockerPluginPath, "terraform-provider-docker_v2.25.0", now.Add(-31*24*time.Hour))
		addPluginFile(t, fs, dockerPluginPath, "LICENSE", now.Add(-32*24*time.Hour))
		addPluginFile(t, fs, dockerPluginPath, "README.md", now.Add(-33*24*time.Hour))

		// when
		terraform.CleanStaleTerraformPlugins(ctx, cachePath, fs, now, logger)

		// then
		diffFileSystem(t, fs)
	})

	t.Run("one plugin file is touched", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		fs, logger := prepare()

		// given
		addPluginFile(t, fs, coderPluginPath, "terraform-provider-coder_v0.11.1", now.Add(-63*24*time.Hour))
		addPluginFile(t, fs, coderPluginPath, "LICENSE", now.Add(-33*24*time.Hour))
		addPluginFile(t, fs, coderPluginPath, "README.md", now.Add(-31*24*time.Hour))
		addPluginFolder(t, fs, coderPluginPath, "new_folder", now.Add(-43*24*time.Hour))
		addPluginFile(t, fs, coderPluginPath, filepath.Join("new_folder", "foobar.tf"), now.Add(-4*time.Hour)) // touched

		addPluginFile(t, fs, dockerPluginPath, "terraform-provider-docker_v2.25.0", now.Add(-31*24*time.Hour))
		addPluginFile(t, fs, dockerPluginPath, "LICENSE", now.Add(-2*time.Hour)) // also touched
		addPluginFile(t, fs, dockerPluginPath, "README.md", now.Add(-33*24*time.Hour))

		// when
		terraform.CleanStaleTerraformPlugins(ctx, cachePath, fs, now, logger)

		// then
		diffFileSystem(t, fs)
	})
}

func addPluginFile(t *testing.T, fs afero.Fs, pluginPath string, resourcePath string, mtime time.Time) {
	err := fs.MkdirAll(filepath.Join(cachePath, pluginPath), 0o755)
	require.NoError(t, err, "can't create test folder for plugin file")

	err = fs.Chtimes(filepath.Join(cachePath, pluginPath), now, mtime)
	require.NoError(t, err, "can't set times")

	err = afero.WriteFile(fs, filepath.Join(cachePath, pluginPath, resourcePath), []byte("foo"), 0o644)
	require.NoError(t, err, "can't create test file")

	err = fs.Chtimes(filepath.Join(cachePath, pluginPath, resourcePath), now, mtime)
	require.NoError(t, err, "can't set times")

	// as creating a file will update mtime of parent, we also want to
	// set the mtime of parent to match that of the new child.
	parent, _ := filepath.Split(filepath.Join(cachePath, pluginPath, resourcePath))
	parentInfo, err := fs.Stat(parent)
	require.NoError(t, err, "can't stat parent")
	if parentInfo.ModTime().After(mtime) {
		require.NoError(t, fs.Chtimes(parent, now, mtime), "can't set mtime of parent to match child")
	}
}

func addPluginFolder(t *testing.T, fs afero.Fs, pluginPath string, folderPath string, mtime time.Time) {
	err := fs.MkdirAll(filepath.Join(cachePath, pluginPath, folderPath), 0o755)
	require.NoError(t, err, "can't create plugin folder")

	err = fs.Chtimes(filepath.Join(cachePath, pluginPath, folderPath), now, mtime)
	require.NoError(t, err, "can't set times")
}

func diffFileSystem(t *testing.T, fs afero.Fs) {
	actual := dumpFileSystem(t, fs)

	partialName := strings.Join(strings.Split(t.Name(), "/")[1:], "_")
	goldenFile := filepath.Join("testdata", "cleanup-stale-plugins", partialName+".txt.golden")
	if *updateGoldenFiles {
		err := os.MkdirAll(filepath.Dir(goldenFile), 0o755)
		require.NoError(t, err, "want no error creating golden file directory")

		err = os.WriteFile(goldenFile, actual, 0o600)
		require.NoError(t, err, "want no error creating golden file")
		return
	}

	want, err := os.ReadFile(goldenFile)
	require.NoError(t, err, "open golden file, run \"make gen/golden-files\" and commit the changes")
	assert.Empty(t, cmp.Diff(want, actual), "golden file mismatch (-want +got): %s, run \"make gen/golden-files\", verify and commit the changes", goldenFile)
}

func dumpFileSystem(t *testing.T, fs afero.Fs) []byte {
	var buffer bytes.Buffer
	err := afero.Walk(fs, "/", func(path string, info os.FileInfo, err error) error {
		_, _ = buffer.WriteString(path)
		_ = buffer.WriteByte(' ')
		if info.IsDir() {
			_ = buffer.WriteByte('d')
		} else {
			_ = buffer.WriteByte('f')
		}
		_ = buffer.WriteByte('\n')
		return nil
	})
	require.NoError(t, err, "can't dump the file system")
	return buffer.Bytes()
}
