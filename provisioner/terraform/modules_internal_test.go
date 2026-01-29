package terraform

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

		archive, skipped, err := GetModulesArchive(os.DirFS(filepath.Join("testdata", "modules-source-caching")))
		require.NoError(t, err)
		require.Len(t, skipped, 0)

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

		archive, skipped, err := GetModulesArchive(afero.NewIOFS(root))
		require.NoError(t, err)
		require.Len(t, skipped, 0)
		require.Equal(t, []byte{}, archive)
	})

	t.Run("ModulesTooLarge", func(t *testing.T) {
		t.Parallel()

		memFS := moduleArchiveFS(t, map[string]moduleDef{
			"small": {
				payload: []byte("small module content"),
			},
			"large": {
				payload: bytes.Repeat([]byte("A"), 10000),
			},
		})
		archive, skipped, err := GetModulesArchiveWithLimit(memFS, 5000)
		require.NoError(t, err)
		require.Len(t, skipped, 1)
		require.Equal(t, skipped[0], "large:large")

		fmt.Println(archive)
	})
}

type moduleDef struct {
	payload []byte
}

func moduleArchiveFS(t *testing.T, defs map[string]moduleDef) fs.FS {
	memFS := afero.NewMemMapFs()
	modRoot := ".terraform/modules"
	err := memFS.MkdirAll(modRoot, 0o755)
	require.NoError(t, err)

	mods := []*module{}
	for name, def := range defs {
		modDir := filepath.Join(modRoot, name)
		err = memFS.Mkdir(modDir, 0o755)
		require.NoError(t, err)

		f, err := memFS.Create(filepath.Join(modDir, "payload"))
		require.NoError(t, err)
		_, err = f.Write(def.payload)
		require.NoError(t, err)
		f.Close()

		mods = append(mods, &module{
			Source:  name,
			Version: "v0.1.0",
			Key:     name,
			Dir:     modDir,
		})
	}

	data, _ := json.Marshal(modulesFile{
		Modules: mods,
	})
	jm, err := memFS.Create(filepath.Join(modRoot, "modules.json"))
	require.NoError(t, err)
	_, err = jm.Write(data)
	require.NoError(t, err)
	jm.Close()

	return afero.NewIOFS(memFS)
}
