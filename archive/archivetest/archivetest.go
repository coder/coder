package archivetest
import (
	"errors"
	"archive/tar"
	"archive/zip"
	"bytes"
	_ "embed"
	"io"
	"testing"
	"time"
	"github.com/stretchr/testify/require"
)
//go:embed testdata/test.tar
var testTarFileBytes []byte
//go:embed testdata/test.zip
var testZipFileBytes []byte
// TestTarFileBytes returns the content of testdata/test.tar
func TestTarFileBytes() []byte {
	return append([]byte{}, testTarFileBytes...)
}
// TestZipFileBytes returns the content of testdata/test.zip
func TestZipFileBytes() []byte {
	return append([]byte{}, testZipFileBytes...)
}
// AssertSampleTarfile compares the content of tarBytes against testdata/test.tar.
func AssertSampleTarFile(t *testing.T, tarBytes []byte) {
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
		require.Equal(t, ArchiveRefTime(t).UTC(), hdr.ModTime.UTC())
		switch hdr.Name {
		case "test/":
			require.Equal(t, hdr.Typeflag, byte(tar.TypeDir))
		case "test/hello.txt":
			require.Equal(t, hdr.Typeflag, byte(tar.TypeReg))
			bs, err := io.ReadAll(tr)
			if err != nil && !errors.Is(err, io.EOF) {
				require.NoError(t, err)
			}
			require.Equal(t, "hello", string(bs))
		case "test/dir/":
			require.Equal(t, hdr.Typeflag, byte(tar.TypeDir))
		case "test/dir/world.txt":
			require.Equal(t, hdr.Typeflag, byte(tar.TypeReg))
			bs, err := io.ReadAll(tr)
			if err != nil && !errors.Is(err, io.EOF) {
				require.NoError(t, err)
			}
			require.Equal(t, "world", string(bs))
		default:
			require.Failf(t, "unexpected file in tar", hdr.Name)
		}
	}
}
// AssertSampleZipFile compares the content of zipBytes against testdata/test.zip.
func AssertSampleZipFile(t *testing.T, zipBytes []byte) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	require.NoError(t, err)
	for _, f := range zr.File {
		// Note: ignoring timezones here.
		require.Equal(t, ArchiveRefTime(t).UTC(), f.Modified.UTC())
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
func ArchiveRefTime(t *testing.T) time.Time {
	locMST, err := time.LoadLocation("MST")
	require.NoError(t, err, "failed to load MST timezone")
	return time.Date(2006, 1, 2, 3, 4, 5, 0, locMST)
}
