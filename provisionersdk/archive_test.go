package provisionersdk_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/provisionersdk"
)

func TestTar(t *testing.T) {
	t.Parallel()

	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	t.Run("NoFollowSymlink", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		file, err := os.CreateTemp(dir, "*.tf")
		require.NoError(t, err)
		_ = file.Close()

		// If we follow symlinks, Tar would fail.
		// See https://github.com/coder/coder/issues/5677.
		err = os.Symlink("no-exists", filepath.Join(dir, "link"))
		require.NoError(t, err)

		err = provisionersdk.Tar(io.Discard, log, dir, 1024*1024)
		require.NoError(t, err)
	})
	t.Run("HeaderBreakLimit", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		file, err := os.CreateTemp(dir, "*.tf")
		require.NoError(t, err)
		_ = file.Close()
		// A header is 512 bytes
		err = provisionersdk.Tar(io.Discard, log, dir, 100)
		require.Error(t, err)
	})
	t.Run("HeaderAndContent", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		file, err := os.CreateTemp(dir, "*.tf")
		require.NoError(t, err)
		_, _ = file.Write(make([]byte, 100))
		_ = file.Close()
		// Pay + header is 1024 bytes (padding)
		err = provisionersdk.Tar(io.Discard, log, dir, 1025)
		require.NoError(t, err)

		// Limit is 1 byte too small (n == limit is a failure, must be under)
		err = provisionersdk.Tar(io.Discard, log, dir, 1024)
		require.Error(t, err)
	})

	t.Run("NoTF", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		file, err := os.CreateTemp(dir, "")
		require.NoError(t, err)
		_ = file.Close()
		err = provisionersdk.Tar(io.Discard, log, dir, 1024)
		require.Error(t, err)
	})
	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		file, err := os.CreateTemp(dir, "*.tf")
		require.NoError(t, err)
		_ = file.Close()
		err = provisionersdk.Tar(io.Discard, log, dir, 1024)
		require.NoError(t, err)
	})
	t.Run("ValidJSON", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		file, err := os.CreateTemp(dir, "*.tf.json")
		require.NoError(t, err)
		_ = file.Close()
		err = provisionersdk.Tar(io.Discard, log, dir, 1024)
		require.NoError(t, err)
	})
	t.Run("HiddenFiles", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		type file struct {
			Name     string
			Archives bool
		}
		files := []*file{
			{
				Name:     "*.tf",
				Archives: true,
			}, {
				Name:     ".*",
				Archives: false,
			}, {
				Name:     "./testing/.test/*.tf",
				Archives: false,
			}, {
				Name:     "./testing/asd.*",
				Archives: true,
			}, {
				Name:     ".terraform/.*",
				Archives: false,
			}, {
				Name:     "example/.terraform/*",
				Archives: false,
			}, {
				Name:     ".terraform.lock.hcl",
				Archives: true,
			}, {
				Name:     "example/.terraform.lock.hcl",
				Archives: true,
			}, {
				Name:     ".terraform/.terraform.lock.hcl",
				Archives: false,
			}, {
				Name:     "terraform.tfstate",
				Archives: false,
			}, {
				Name:     "terraform.tfvars",
				Archives: false,
			}, {
				Name:     "terraform.tfvars.json",
				Archives: false,
			}, {
				Name:     "*.auto.tfvars",
				Archives: false,
			}, {
				Name:     "*.auto.tfvars.json",
				Archives: false,
			},
		}
		for _, file := range files {
			newDir := dir
			file.Name = filepath.FromSlash(file.Name)
			if filepath.Base(file.Name) != file.Name {
				newDir = filepath.Join(newDir, filepath.Dir(file.Name))
				err := os.MkdirAll(newDir, 0o755)
				require.NoError(t, err)
				file.Name = filepath.Base(file.Name)
			}
			if strings.Contains(file.Name, "*") {
				tmpFile, err := os.CreateTemp(newDir, file.Name)
				require.NoError(t, err)
				_ = tmpFile.Close()
				file.Name, err = filepath.Rel(dir, tmpFile.Name())
				require.NoError(t, err)
			} else {
				name := filepath.Join(newDir, file.Name)
				err := os.WriteFile(name, []byte{}, 0o600)
				require.NoError(t, err)
				file.Name, err = filepath.Rel(dir, name)
				require.NoError(t, err)
			}
		}
		archive := new(bytes.Buffer)
		// Headers are chonky so raise the limit to something reasonable
		err := provisionersdk.Tar(archive, log, dir, 1024<<3)
		require.NoError(t, err)
		dir = t.TempDir()
		err = provisionersdk.Untar(dir, archive)
		require.NoError(t, err)
		for _, file := range files {
			_, err = os.Stat(filepath.Join(dir, file.Name))
			if file.Archives {
				require.NoError(t, err, "stat %q, got error: %+v", file.Name, err)
			} else {
				require.ErrorIs(t, err, os.ErrNotExist, "stat %q, expected ErrNotExist, got: %+v", file.Name, err)
			}
		}
	})
}

func TestUntar(t *testing.T) {
	t.Parallel()

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()

		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

		dir := t.TempDir()
		file, err := os.CreateTemp(dir, "*.tf")
		require.NoError(t, err)
		_ = file.Close()

		archive := new(bytes.Buffer)
		err = provisionersdk.Tar(archive, log, dir, 1024)
		require.NoError(t, err)

		dir = t.TempDir()
		err = provisionersdk.Untar(dir, archive)
		require.NoError(t, err)

		_, err = os.Stat(filepath.Join(dir, filepath.Base(file.Name())))
		require.NoError(t, err)
	})

	t.Run("Overwrite", func(t *testing.T) {
		t.Parallel()

		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

		dir1 := t.TempDir()
		dir2 := t.TempDir()

		// 1. Create directory with .tf file.
		file, err := os.CreateTemp(dir1, "*.tf")
		require.NoError(t, err)
		_ = file.Close()

		err = os.WriteFile(file.Name(), []byte("# ab"), 0o600)
		require.NoError(t, err)

		archive := new(bytes.Buffer)

		// 2. Build tar archive.
		err = provisionersdk.Tar(archive, log, dir1, 4096)
		require.NoError(t, err)

		// 3. Untar to the second location.
		err = provisionersdk.Untar(dir2, archive)
		require.NoError(t, err)

		// 4. Modify the .tf file
		err = os.WriteFile(file.Name(), []byte("# c"), 0o600)
		require.NoError(t, err)

		// 5. Build tar archive with modified .tf file
		err = provisionersdk.Tar(archive, log, dir1, 4096)
		require.NoError(t, err)

		// 6. Untar to a second location.
		err = provisionersdk.Untar(dir2, archive)
		require.NoError(t, err)

		// Verify if the file has been fully overwritten
		content, err := os.ReadFile(filepath.Join(dir2, filepath.Base(file.Name())))
		require.NoError(t, err)
		require.Equal(t, "# c", string(content))
	})
}
