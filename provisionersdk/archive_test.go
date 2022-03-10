package provisionersdk_test

import (
	"io/ioutil"
	"testing"

	"github.com/coder/coder/provisionersdk"
	"github.com/stretchr/testify/require"
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
