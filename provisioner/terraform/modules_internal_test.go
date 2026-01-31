package terraform

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
		require.Equal(t, "large:large", skipped[0])

		// Verify small module is in the archive
		tarfs := archivefs.FromTarReader(bytes.NewBuffer(archive))
		_, err = fs.ReadFile(tarfs, ".terraform/modules/small/payload")
		require.NoError(t, err, "small module should be included")
	})

	// TestModulePackingPrioritizesSmallest verifies that when space is limited,
	// smaller modules are included first to maximize the number of modules archived.
	t.Run("PackingPrioritizesSmallest", func(t *testing.T) {
		t.Parallel()

		// Create modules of varying sizes. With a limit that can fit
		// small + medium but not large, we should see small and medium included.
		memFS := moduleArchiveFS(t, map[string]moduleDef{
			"small": {
				payload: bytes.Repeat([]byte("S"), 500),
			},
			"medium": {
				payload: bytes.Repeat([]byte("M"), 1500),
			},
			"large": {
				payload: bytes.Repeat([]byte("L"), 5000),
			},
		})

		// Estimate: each module needs ~512 (dir) + 512 (file header) + content + padding
		// small: ~1536 bytes, medium: ~2560 bytes, large: ~6144 bytes
		// Plus modules.json overhead (~1024) and tar end blocks (1024).
		// Set limit to fit small + medium + overhead but not large.
		archive, skipped, err := GetModulesArchiveWithLimit(memFS, 8000)
		require.NoError(t, err)

		require.Len(t, skipped, 1, "only the large module should be skipped")
		require.Equal(t, "large:large", skipped[0])

		// Verify correct modules are in archive
		tarfs := archivefs.FromTarReader(bytes.NewBuffer(archive))
		_, err = fs.ReadFile(tarfs, ".terraform/modules/small/payload")
		require.NoError(t, err, "small module should be included")
		_, err = fs.ReadFile(tarfs, ".terraform/modules/medium/payload")
		require.NoError(t, err, "medium module should be included")
		_, err = fs.ReadFile(tarfs, ".terraform/modules/large/payload")
		require.Error(t, err, "large module should NOT be included")
	})

	// TestModulePackingAllFit verifies all modules are included when under budget.
	t.Run("PackingAllFit", func(t *testing.T) {
		t.Parallel()

		memFS := moduleArchiveFS(t, map[string]moduleDef{
			"mod1": {payload: []byte("module one")},
			"mod2": {payload: []byte("module two")},
			"mod3": {payload: []byte("module three")},
		})

		// Large limit - everything should fit
		archive, skipped, err := GetModulesArchiveWithLimit(memFS, 100000)
		require.NoError(t, err)
		require.Empty(t, skipped, "no modules should be skipped")

		tarfs := archivefs.FromTarReader(bytes.NewBuffer(archive))
		_, err = fs.ReadFile(tarfs, ".terraform/modules/mod1/payload")
		require.NoError(t, err)
		_, err = fs.ReadFile(tarfs, ".terraform/modules/mod2/payload")
		require.NoError(t, err)
		_, err = fs.ReadFile(tarfs, ".terraform/modules/mod3/payload")
		require.NoError(t, err)
	})

	// TestModulePackingNoneFit verifies behavior when no modules fit.
	t.Run("PackingNoneFit", func(t *testing.T) {
		t.Parallel()

		memFS := moduleArchiveFS(t, map[string]moduleDef{
			"mod1": {payload: bytes.Repeat([]byte("X"), 2000)},
			"mod2": {payload: bytes.Repeat([]byte("Y"), 3000)},
		})

		// Set limit that's enough for modules.json but not for the modules themselves
		// modules.json needs ~512 header + content + padding + 1024 end blocks
		archive, skipped, err := GetModulesArchiveWithLimit(memFS, 2500)
		require.NoError(t, err)
		require.Len(t, skipped, 2, "both modules should be skipped")

		// Archive should just contain modules.json (empty means no module content)
		require.True(t, len(archive) == 0 || len(archive) < 2500,
			"archive should be empty or minimal when no modules fit")
	})

	// TestModulePackingEdgeCaseExactFit tests when a module exactly fits the remaining space.
	// The second module should be skipped, because the first module is perfect.
	t.Run("PackingEdgeCaseExactFit", func(t *testing.T) {
		t.Parallel()

		originalDef := map[string]moduleDef{
			"exact": {payload: bytes.Repeat([]byte("E"), 1000)},
		}
		// Create a single module and measure its actual archive size
		memFS := moduleArchiveFS(t, originalDef)

		// First, get the actual size with no limit
		archive, skipped, err := GetModulesArchiveWithLimit(memFS, 100000)
		require.NoError(t, err)
		require.Empty(t, skipped)
		actualSize := int64(len(archive))

		originalDef["extra"] = moduleDef{payload: bytes.Repeat([]byte("X"), 1001)}
		memFS = moduleArchiveFS(t, originalDef)

		// Now test with exact size - should just fit
		archive, skipped, err = GetModulesArchiveWithLimit(memFS, actualSize)
		require.NoError(t, err)
		require.Len(t, skipped, 1)
		require.Equal(t, skipped[0], "extra:extra", "extra module should be skipped")
		require.Equal(t, actualSize, int64(len(archive)))
	})

	// TestModulePackingMultipleSkipped verifies correct behavior when multiple
	// large modules must be skipped.
	t.Run("PackingMultipleSkipped", func(t *testing.T) {
		t.Parallel()

		memFS := moduleArchiveFS(t, map[string]moduleDef{
			"tiny":   {payload: []byte("t")},
			"small":  {payload: bytes.Repeat([]byte("S"), 200)},
			"large1": {payload: bytes.Repeat([]byte("L"), 5000)},
			"large2": {payload: bytes.Repeat([]byte("L"), 6000)},
			"large3": {payload: bytes.Repeat([]byte("L"), 7000)},
		})

		// Set limit to fit tiny + small + overhead but not the large ones
		// tiny: ~1536, small: ~1536, overhead (modules.json + tar end): ~3072
		archive, skipped, err := GetModulesArchiveWithLimit(memFS, 7000)
		require.NoError(t, err)

		require.Len(t, skipped, 3, "all three large modules should be skipped")

		tarfs := archivefs.FromTarReader(bytes.NewBuffer(archive))
		_, err = fs.ReadFile(tarfs, ".terraform/modules/tiny/payload")
		require.NoError(t, err, "tiny module should be included")
		_, err = fs.ReadFile(tarfs, ".terraform/modules/small/payload")
		require.NoError(t, err, "small module should be included")
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
