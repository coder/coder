package provisionersdk_test

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
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

	t.Run("SkipsSymlinks", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		// Create a real .tf file.
		err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("# real"), 0o600)
		require.NoError(t, err)

		// Create a symlink to a file and a dangling symlink.
		err = os.Symlink(filepath.Join(dir, "main.tf"), filepath.Join(dir, "linked.tf"))
		require.NoError(t, err)
		err = os.Symlink("no-target", filepath.Join(dir, "dangling"))
		require.NoError(t, err)

		var buf bytes.Buffer
		err = provisionersdk.Tar(&buf, log, dir, 1024*1024)
		require.NoError(t, err)

		// Read the archive and verify no symlink entries exist.
		tr := tar.NewReader(&buf)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			require.NotEqual(t, tar.TypeSymlink, hdr.Typeflag,
				"symlink entry %q should not be in archive", hdr.Name)
		}
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

	t.Run("PathTraversal", func(t *testing.T) {
		t.Parallel()

		// Create a tar archive with a path traversal entry.
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		// Write a file with a "../" prefix to attempt traversal.
		err := tw.WriteHeader(&tar.Header{
			Name:     "../etc/passwd",
			Mode:     0o644,
			Size:     4,
			Typeflag: tar.TypeReg,
		})
		require.NoError(t, err)
		_, err = tw.Write([]byte("evil"))
		require.NoError(t, err)
		// Also write a valid file to confirm extraction still works.
		err = tw.WriteHeader(&tar.Header{
			Name:     "safe.txt",
			Mode:     0o644,
			Size:     4,
			Typeflag: tar.TypeReg,
		})
		require.NoError(t, err)
		_, err = tw.Write([]byte("good"))
		require.NoError(t, err)
		require.NoError(t, tw.Close())

		dir := t.TempDir()
		err = provisionersdk.Untar(dir, &buf)
		require.NoError(t, err)

		// The traversal file must not exist anywhere.
		_, err = os.Stat(filepath.Join(dir, "..", "etc", "passwd"))
		require.ErrorIs(t, err, os.ErrNotExist)

		// The safe file should exist.
		_, err = os.Stat(filepath.Join(dir, "safe.txt"))
		require.NoError(t, err)
	})

	t.Run("SkipsSymlinks", func(t *testing.T) {
		t.Parallel()

		// Create a tar archive containing a symlink entry.
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		err := tw.WriteHeader(&tar.Header{
			Name:     "link",
			Linkname: "/etc/passwd",
			Typeflag: tar.TypeSymlink,
		})
		require.NoError(t, err)
		// Also add a hard link entry.
		err = tw.WriteHeader(&tar.Header{
			Name:     "hardlink",
			Linkname: "link",
			Typeflag: tar.TypeLink,
		})
		require.NoError(t, err)
		// Add a regular file so the archive is not empty.
		err = tw.WriteHeader(&tar.Header{
			Name:     "real.txt",
			Mode:     0o644,
			Size:     5,
			Typeflag: tar.TypeReg,
		})
		require.NoError(t, err)
		_, err = tw.Write([]byte("hello"))
		require.NoError(t, err)
		require.NoError(t, tw.Close())

		dir := t.TempDir()
		err = provisionersdk.Untar(dir, &buf)
		require.NoError(t, err)

		// Symlink and hardlink must not exist.
		_, err = os.Lstat(filepath.Join(dir, "link"))
		require.ErrorIs(t, err, os.ErrNotExist)
		_, err = os.Lstat(filepath.Join(dir, "hardlink"))
		require.ErrorIs(t, err, os.ErrNotExist)

		// Regular file should be extracted.
		_, err = os.Stat(filepath.Join(dir, "real.txt"))
		require.NoError(t, err)
	})

	t.Run("ZeroModeFallback", func(t *testing.T) {
		t.Parallel()

		// Create a tar with a file that has zero mode bits.
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		err := tw.WriteHeader(&tar.Header{
			Name:     "nomode.txt",
			Mode:     0,
			Size:     3,
			Typeflag: tar.TypeReg,
		})
		require.NoError(t, err)
		_, err = tw.Write([]byte("abc"))
		require.NoError(t, err)
		require.NoError(t, tw.Close())

		dir := t.TempDir()
		err = provisionersdk.Untar(dir, &buf)
		require.NoError(t, err)

		// The file should exist and be readable.
		content, err := os.ReadFile(filepath.Join(dir, "nomode.txt"))
		require.NoError(t, err)
		require.Equal(t, "abc", string(content))

		// The file mode should be the fallback (0o600).
		info, err := os.Stat(filepath.Join(dir, "nomode.txt"))
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	})
}
