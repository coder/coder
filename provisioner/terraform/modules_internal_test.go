package terraform

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	archivefs "github.com/coder/coder/v2/archive/fs"
)

// The .tar archive is different on Windows because of git converting LF line
// endings to CRLF line endings, so many of the assertions in this test are
// platform specific.
func TestGetModulesArchive(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		archive, err := GetModulesArchive(os.DirFS(filepath.Join("testdata", "modules-source-caching")))
		require.NoError(t, err)

		// Check that all of the files it should contain are correct
		b := bytes.NewBuffer(archive)
		tarfs := archivefs.FromTarReader(b)

		content, err := fs.ReadFile(tarfs, ".terraform/modules/modules.json")
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(string(content), `{"Modules":[{"Key":"","Source":"","Dir":"."},`))

		dirFiles, err := fs.ReadDir(tarfs, ".terraform/modules/example_module")
		require.NoError(t, err)
		require.Len(t, dirFiles, 1)
		require.Equal(t, "main.tf", dirFiles[0].Name())

		content, err = fs.ReadFile(tarfs, ".terraform/modules/example_module/main.tf")
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(string(content), "terraform {"))
		if runtime.GOOS != "windows" {
			require.Len(t, content, 3691)
		} else {
			require.Len(t, content, 3812)
		}

		_, err = fs.ReadFile(tarfs, ".terraform/modules/stuff_that_should_not_be_included/nothing.txt")
		require.Error(t, err)

		// It should always be byte-identical to optimize storage
		hashBytes := sha256.Sum256(archive)
		hash := hex.EncodeToString(hashBytes[:])
		if runtime.GOOS != "windows" {
			require.Equal(t, "edcccdd4db68869552542e66bad87a51e2e455a358964912805a32b06123cb5c", hash)
		} else {
			require.Equal(t, "67027a27452d60ce2799fcfd70329c185f9aee7115b0944e3aa00b4776be9d92", hash)
		}
	})

	t.Run("EmptyDirectory", func(t *testing.T) {
		t.Parallel()

		root := afero.NewMemMapFs()
		afero.WriteFile(root, ".terraform/modules/modules.json", []byte(`{"Modules":[{"Key":"","Source":"","Dir":"."}]}`), 0o644)

		archive, err := GetModulesArchive(afero.NewIOFS(root))
		require.NoError(t, err)
		require.Equal(t, []byte{}, archive)
	})
}
