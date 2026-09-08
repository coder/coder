package desktopruntime

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

type entry struct {
	name     string
	typeflag byte
	mode     int64
	body     string
	linkname string
}

func makeArchive(t *testing.T, entries []entry) []byte {
	t.Helper()
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Typeflag: e.typeflag,
			Mode:     e.mode,
			Linkname: e.linkname,
		}
		if e.typeflag == tar.TypeReg {
			hdr.Size = int64(len(e.body))
		}
		require.NoError(t, tw.WriteHeader(hdr))
		if e.typeflag == tar.TypeReg {
			_, err := tw.Write([]byte(e.body))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())

	var out bytes.Buffer
	zw, err := zstd.NewWriter(&out)
	require.NoError(t, err)
	_, err = zw.Write(tarBuf.Bytes())
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return out.Bytes()
}

func validEntries() []entry {
	return []entry{
		{name: "./", typeflag: tar.TypeDir, mode: 0o755},
		{name: "./bin/", typeflag: tar.TypeDir, mode: 0o755},
		{name: "./bin/Xvnc", typeflag: tar.TypeReg, mode: 0o755, body: "#!/bin/sh\nexit 0\n"},
		{name: "./bin/xkbcomp", typeflag: tar.TypeReg, mode: 0o755, body: "#!/bin/sh\n"},
		{name: "./bin/Xvnc-alias", typeflag: tar.TypeSymlink, mode: 0o777, linkname: "Xvnc"},
		{name: "./share/xkb/rules/evdev", typeflag: tar.TypeReg, mode: 0o644, body: "! model = keycodes\n"},
		{name: "./manifest.json", typeflag: tar.TypeReg, mode: 0o644, body: "{}"},
	}
}

func TestExtract(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		dest := t.TempDir()
		require.NoError(t, extract(context.Background(), makeArchive(t, validEntries()), dest))
		require.NoError(t, Validate(dest))

		info, err := os.Stat(filepath.Join(dest, "share", "xkb", "rules", "evdev"))
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o644), info.Mode().Perm())

		link, err := os.Readlink(filepath.Join(dest, "bin", "Xvnc-alias"))
		require.NoError(t, err)
		require.Equal(t, "Xvnc", link)
	})

	t.Run("RejectsPathTraversal", func(t *testing.T) {
		t.Parallel()
		dest := t.TempDir()
		entries := append(validEntries(), entry{
			name: "../escape", typeflag: tar.TypeReg, mode: 0o644, body: "x",
		})
		err := extract(context.Background(), makeArchive(t, entries), dest)
		require.ErrorContains(t, err, "escapes destination")
		_, statErr := os.Stat(filepath.Join(filepath.Dir(dest), "escape"))
		require.ErrorIs(t, statErr, os.ErrNotExist)
	})

	t.Run("RejectsEscapingSymlink", func(t *testing.T) {
		t.Parallel()
		dest := t.TempDir()
		entries := append(validEntries(), entry{
			name: "./bin/evil", typeflag: tar.TypeSymlink, mode: 0o777, linkname: "../../outside",
		})
		err := extract(context.Background(), makeArchive(t, entries), dest)
		require.ErrorContains(t, err, "escapes runtime root")
	})

	t.Run("RejectsAbsoluteSymlink", func(t *testing.T) {
		t.Parallel()
		dest := t.TempDir()
		entries := append(validEntries(), entry{
			name: "./bin/evil", typeflag: tar.TypeSymlink, mode: 0o777, linkname: "/etc/passwd",
		})
		err := extract(context.Background(), makeArchive(t, entries), dest)
		require.ErrorContains(t, err, "absolute symlink")
	})

	t.Run("RejectsUnsupportedType", func(t *testing.T) {
		t.Parallel()
		dest := t.TempDir()
		entries := append(validEntries(), entry{
			name: "./bin/hard", typeflag: tar.TypeLink, mode: 0o644, linkname: "bin/Xvnc",
		})
		err := extract(context.Background(), makeArchive(t, entries), dest)
		require.ErrorContains(t, err, "unsupported tar entry type")
	})

	t.Run("Canceled", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := extract(ctx, makeArchive(t, validEntries()), t.TempDir())
		require.ErrorIs(t, err, context.Canceled)
	})
}

func TestValidate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.Error(t, Validate(dir), "missing Xvnc must fail")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "bin"), 0o755))
	xvnc := filepath.Join(dir, XvncPath)
	require.NoError(t, os.WriteFile(xvnc, []byte("#!/bin/sh\n"), 0o600))
	require.ErrorContains(t, Validate(dir), "not an executable file")

	require.NoError(t, os.Chmod(xvnc, 0o755))
	require.NoError(t, Validate(dir))
}

func TestUnpackNotAvailable(t *testing.T) {
	t.Parallel()
	if Available() {
		t.Skip("runtime is embedded in this build")
	}
	_, err := Unpack(context.Background(), t.TempDir())
	require.ErrorIs(t, err, ErrNotAvailable)
	require.Empty(t, SHA256())
	require.Empty(t, ArchiveName)
}

func TestDefaultCacheDir(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/tmp/xdg-test")
	dir, err := DefaultCacheDir()
	require.NoError(t, err)
	require.Equal(t, filepath.Join("/tmp/xdg-test", "coder"), dir)

	t.Setenv("XDG_CACHE_HOME", "")
	dir, err = DefaultCacheDir()
	require.NoError(t, err)
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, ".cache", "coder"), dir)
}
