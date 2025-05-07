package terraform

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	archivefs "github.com/coder/coder/v2/archive/fs"
)

// The .tar archive is different on Windows because of git converting LF line
// endings to CRLF line endings, so many of the assertions in this test are
// platform specific.
func TestGetModulesArchive(t *testing.T) {
	t.Parallel()

	archive, err := getModulesArchive(filepath.Join("testdata", "modules-source-caching"))
	require.NoError(t, err)

	// Check that all of the files it should contain are correct
	r := bytes.NewBuffer(archive)
	tarfs := archivefs.FromTarReader(r)
	content, err := fs.ReadFile(tarfs, ".terraform/modules/example_module/main.tf")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(content), "terraform {"))
	if runtime.GOOS != "windows" {
		require.Len(t, content, 3691)
	} else {
		require.Len(t, content, 3812)
	}

	// It should always be byte-identical to optimize storage
	hashBytes := sha256.Sum256(archive)
	hash := hex.EncodeToString(hashBytes[:])
	if runtime.GOOS != "windows" {
		require.Equal(t, "05d2994c1a50ce573fe2c2b29507e5131ba004d15812d8bb0a46dc732f3211f5", hash)
	} else {
		require.Equal(t, "0001fc95ac0ac18188931db2ef28c42f51919ee24bc18482fab38d1ea9c7a4e8", hash)
	}
}
