package provisionersdk_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/provisionersdk"
)

func TestTar(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file, err := ioutil.TempFile(dir, "")
	require.NoError(t, err)
	_ = file.Close()
	_, err = provisionersdk.Tar(dir)
	require.NoError(t, err)
}

func TestUntar(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file, err := ioutil.TempFile(dir, "")
	require.NoError(t, err)
	_ = file.Close()
	archive, err := provisionersdk.Tar(dir)
	require.NoError(t, err)
	dir = t.TempDir()
	err = provisionersdk.Untar(dir, archive)
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, file.Name()))
	require.NoError(t, err)
}
