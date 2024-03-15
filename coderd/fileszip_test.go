package coderd_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/testutil"
)

func TestCreateTarFromZip(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("skipping this test on non-Linux platform")
	}

	// Read a zip file we prepared earlier
	ctx := testutil.Context(t, testutil.WaitShort)
	zipBytes, err := os.ReadFile(filepath.Join("testdata", "test.zip"))
	require.NoError(t, err, "failed to read sample zip file")
	// Assert invariant
	assertSampleZipFile(t, zipBytes)

	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	require.NoError(t, err, "failed to parse sample zip file")

	tarBytes, err := coderd.CreateTarFromZip(zr)
	require.NoError(t, err, "failed to convert zip to tar")

	assertSampleTarFile(t, tarBytes)

	tempDir := t.TempDir()
	tempFilePath := filepath.Join(tempDir, "test.tar")
	err = os.WriteFile(tempFilePath, tarBytes, 0o600)
	require.NoError(t, err, "failed to write converted tar file")

	cmd := exec.CommandContext(ctx, "tar", "--extract", "--verbose", "--file", tempFilePath, "--directory", tempDir)
	require.NoError(t, cmd.Run(), "failed to extract converted tar file")
	assertExtractedFiles(t, tempDir, true)
}

func TestCreateZipFromTar(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("skipping this test on non-Linux platform")
	}
	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		tarBytes, err := os.ReadFile(filepath.Join(".", "testdata", "test.tar"))
		require.NoError(t, err, "failed to read sample tar file")

		tr := tar.NewReader(bytes.NewReader(tarBytes))
		zipBytes, err := coderd.CreateZipFromTar(tr)
		require.NoError(t, err)

		assertSampleZipFile(t, zipBytes)

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
		zipBytes, err := coderd.CreateZipFromTar(tr)
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
			assert.Equal(t, archiveRefTime(t).UTC(), stat.ModTime().UTC(), "unexpected modtime of %q", path)
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

func assertSampleTarFile(t *testing.T, tarBytes []byte) {
	t.Helper()

	tr := tar.NewReader(bytes.NewReader(tarBytes))
	for {
		hdr, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				return
			}
			require.NoError(t, err)
		}

		// Note: ignoring timezones here.
		require.Equal(t, archiveRefTime(t).UTC(), hdr.ModTime.UTC())

		switch hdr.Name {
		case "test/":
			require.Equal(t, hdr.Typeflag, byte(tar.TypeDir))
		case "test/hello.txt":
			require.Equal(t, hdr.Typeflag, byte(tar.TypeReg))
			bs, err := io.ReadAll(tr)
			if err != nil && !xerrors.Is(err, io.EOF) {
				require.NoError(t, err)
			}
			require.Equal(t, "hello", string(bs))
		case "test/dir/":
			require.Equal(t, hdr.Typeflag, byte(tar.TypeDir))
		case "test/dir/world.txt":
			require.Equal(t, hdr.Typeflag, byte(tar.TypeReg))
			bs, err := io.ReadAll(tr)
			if err != nil && !xerrors.Is(err, io.EOF) {
				require.NoError(t, err)
			}
			require.Equal(t, "world", string(bs))
		default:
			require.Failf(t, "unexpected file in tar", hdr.Name)
		}
	}
}

func assertSampleZipFile(t *testing.T, zipBytes []byte) {
	t.Helper()

	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	require.NoError(t, err)

	for _, f := range zr.File {
		// Note: ignoring timezones here.
		require.Equal(t, archiveRefTime(t).UTC(), f.Modified.UTC())
		switch f.Name {
		case "test/", "test/dir/":
			// directory
		case "test/hello.txt":
			rc, err := f.Open()
			require.NoError(t, err)
			bs, err := io.ReadAll(rc)
			_ = rc.Close()
			require.NoError(t, err)
			require.Equal(t, "hello", string(bs))
		case "test/dir/world.txt":
			rc, err := f.Open()
			require.NoError(t, err)
			bs, err := io.ReadAll(rc)
			_ = rc.Close()
			require.NoError(t, err)
			require.Equal(t, "world", string(bs))
		default:
			require.Failf(t, "unexpected file in zip", f.Name)
		}
	}
}

// archiveRefTime is the Go reference time. The contents of the sample tar and zip files
// in testdata/ all have their modtimes set to the below in some timezone.
func archiveRefTime(t *testing.T) time.Time {
	locMST, err := time.LoadLocation("MST")
	require.NoError(t, err, "failed to load MST timezone")
	return time.Date(2006, 1, 2, 3, 4, 5, 0, locMST)
}
