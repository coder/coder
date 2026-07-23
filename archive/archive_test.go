package archive_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"encoding/binary"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/archive"
	"github.com/coder/coder/v2/archive/archivetest"
	"github.com/coder/coder/v2/testutil"
)

func TestCreateTarFromZip(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("skipping this test on non-Linux platform")
	}

	// Read a zip file we prepared earlier
	ctx := testutil.Context(t, testutil.WaitShort)
	zipBytes := archivetest.TestZipFileBytes()
	// Assert invariant
	archivetest.AssertSampleZipFile(t, zipBytes)

	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	require.NoError(t, err, "failed to parse sample zip file")

	wantTar := archivetest.TestTarFileBytes()
	gotTar, err := archive.CreateTarFromZip(zr, int64(len(wantTar)))
	require.NoError(t, err, "failed to convert zip to tar")

	archivetest.AssertSampleTarFile(t, gotTar)

	tempDir := t.TempDir()
	tempFilePath := filepath.Join(tempDir, "test.tar")
	err = os.WriteFile(tempFilePath, gotTar, 0o600)
	require.NoError(t, err, "failed to write converted tar file")

	cmd := exec.CommandContext(ctx, "tar", "--extract", "--verbose", "--file", tempFilePath, "--directory", tempDir)
	require.NoError(t, cmd.Run(), "failed to extract converted tar file")
	assertExtractedFiles(t, tempDir, true)
}

func buildTestZip(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var zipBytes bytes.Buffer
	zw := zip.NewWriter(&zipBytes)
	for name, contents := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)

		_, err = w.Write([]byte(contents))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())

	return zipBytes.Bytes()
}

func TestCreateTarFromZip_RejectsOversizedAggregateExpansion(t *testing.T) {
	t.Parallel()

	zipBytes := buildTestZip(t, map[string]string{
		"a.txt": strings.Repeat("a", 600),
		"b.txt": strings.Repeat("b", 600),
		"c.txt": strings.Repeat("c", 600),
	})

	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	require.NoError(t, err)

	tarBytes, err := archive.CreateTarFromZip(zr, 1024)
	require.Error(t, err)
	require.Nil(t, tarBytes)
}

func TestCreateTarFromZip_RejectsInvalidZipMetadata(t *testing.T) {
	t.Parallel()

	// Ref: https://github.com/golang/go/blob/go1.24.0/src/archive/zip/struct.go
	corruptZipUncompressedSize := func(t *testing.T, zipBytes []byte, size uint32) []byte {
		t.Helper()

		const (
			directoryHeaderSignature = "PK\x01\x02"
			uncompressedSizeOffset   = 24
		)
		hdrOffset := bytes.Index(zipBytes, []byte(directoryHeaderSignature))
		require.NotEqual(t, -1, hdrOffset, "missing ZIP central directory header")
		corrupted := bytes.Clone(zipBytes)
		sizeBytes := corrupted[hdrOffset+uncompressedSizeOffset : hdrOffset+uncompressedSizeOffset+4]
		binary.LittleEndian.PutUint32(sizeBytes, size)

		return corrupted
	}

	zipBytes := buildTestZip(t, map[string]string{
		"hello.txt": "hello",
	})
	zipBytes = corruptZipUncompressedSize(t, zipBytes, 6)

	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	require.NoError(t, err)

	// Keep the size limit large so this test exercises the invalid
	// ZIP metadata path rather than the tar output limit.
	maxSize := int64(4096)
	tarBytes, err := archive.CreateTarFromZip(zr, maxSize)
	require.ErrorIs(t, err, archive.ErrInvalidZipContent)
	require.Nil(t, tarBytes)
}

func TestCreateTarFromZip_RejectsOversizedTarOverhead(t *testing.T) {
	t.Parallel()

	// Empty files keep the ZIP payload tiny while still forcing tar
	// headers and end-of-archive blocks to consume output budget.
	zipBytes := buildTestZip(t, map[string]string{
		"empty-a.txt": "",
		"empty-b.txt": "",
	})

	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	require.NoError(t, err)

	// Two empty tar entries still need 2 header blocks plus the 2
	// end-of-archive blocks, so the output is 2048 bytes and must
	// exceed this limit.
	tarBytes, err := archive.CreateTarFromZip(zr, 2047)
	require.Error(t, err)
	require.Nil(t, tarBytes)
}

func TestCreateZipFromTar(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("skipping this test on non-Linux platform")
	}
	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		tarBytes := archivetest.TestTarFileBytes()

		tr := tar.NewReader(bytes.NewReader(tarBytes))
		zipBytes, err := archive.CreateZipFromTar(tr, int64(len(tarBytes)))
		require.NoError(t, err)

		archivetest.AssertSampleZipFile(t, zipBytes)

		tempDir := t.TempDir()
		tempFilePath := filepath.Join(tempDir, "test.zip")
		err = os.WriteFile(tempFilePath, zipBytes, 0o600)
		require.NoError(t, err, "failed to write converted zip file")

		ctx := testutil.Context(t, testutil.WaitShort)
		cmd := exec.CommandContext(ctx, "unzip", tempFilePath, "-d", tempDir)
		require.NoError(t, cmd.Run(), "failed to extract converted zip file")

		assertExtractedFiles(t, tempDir, false)
	})

	t.Run("MissingSlashInDirectoryHeader", func(t *testing.T) {
		t.Parallel()

		// Given: a tar archive containing a directory entry that has the directory
		// mode bit set but the name is missing a trailing slash

		var tarBytes bytes.Buffer
		tw := tar.NewWriter(&tarBytes)
		tw.WriteHeader(&tar.Header{
			Name:     "dir",
			Typeflag: tar.TypeDir,
			Size:     0,
		})
		require.NoError(t, tw.Flush())
		require.NoError(t, tw.Close())

		// When: we convert this to a zip
		tr := tar.NewReader(&tarBytes)
		zipBytes, err := archive.CreateZipFromTar(tr, int64(tarBytes.Len()))
		require.NoError(t, err)

		// Then: the resulting zip should contain a corresponding directory
		zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
		require.NoError(t, err)
		for _, zf := range zr.File {
			switch zf.Name {
			case "dir":
				require.Fail(t, "missing trailing slash in directory name")
			case "dir/":
				require.True(t, zf.Mode().IsDir(), "should be a directory")
			default:
				require.Fail(t, "unexpected file in archive")
			}
		}
	})
}

// nolint:revive // this is a control flag but it's in a unit test
func assertExtractedFiles(t *testing.T, dir string, checkModePerm bool) {
	t.Helper()

	_ = filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		relPath := strings.TrimPrefix(path, dir)
		switch relPath {
		case "", "/test.zip", "/test.tar": // ignore
		case "/test":
			stat, err := os.Stat(path)
			assert.NoError(t, err, "failed to stat path %q", path)
			assert.True(t, stat.IsDir(), "expected path %q to be a directory")
			if checkModePerm {
				assert.Equal(t, fs.ModePerm&0o755, stat.Mode().Perm(), "expected mode 0755 on directory")
			}
			assert.Equal(t, archivetest.ArchiveRefTime(t).UTC(), stat.ModTime().UTC(), "unexpected modtime of %q", path)
		case "/test/hello.txt":
			stat, err := os.Stat(path)
			assert.NoError(t, err, "failed to stat path %q", path)
			assert.False(t, stat.IsDir(), "expected path %q to be a file")
			if checkModePerm {
				assert.Equal(t, fs.ModePerm&0o644, stat.Mode().Perm(), "expected mode 0644 on file")
			}
			bs, err := os.ReadFile(path)
			assert.NoError(t, err, "failed to read file %q", path)
			assert.Equal(t, "hello", string(bs), "unexpected content in file %q", path)
		case "/test/dir":
			stat, err := os.Stat(path)
			assert.NoError(t, err, "failed to stat path %q", path)
			assert.True(t, stat.IsDir(), "expected path %q to be a directory")
			if checkModePerm {
				assert.Equal(t, fs.ModePerm&0o755, stat.Mode().Perm(), "expected mode 0755 on directory")
			}
		case "/test/dir/world.txt":
			stat, err := os.Stat(path)
			assert.NoError(t, err, "failed to stat path %q", path)
			assert.False(t, stat.IsDir(), "expected path %q to be a file")
			if checkModePerm {
				assert.Equal(t, fs.ModePerm&0o644, stat.Mode().Perm(), "expected mode 0644 on file")
			}
			bs, err := os.ReadFile(path)
			assert.NoError(t, err, "failed to read file %q", path)
			assert.Equal(t, "world", string(bs), "unexpected content in file %q", path)
		default:
			assert.Fail(t, "unexpected path", relPath)
		}

		return nil
	})
}
