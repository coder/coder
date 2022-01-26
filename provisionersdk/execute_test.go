package provisionersdk_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestExecute(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "terraform")
	cmd := exec.Command("go", "build", "-o", path, "../cmd/provisionerterraform")
	err := cmd.Run()
	require.NoError(t, err)

	client, err := provisionersdk.Execute(context.Background(), path)
	require.NoError(t, err)
	defer client.DRPCConn().Close()
	_, err = client.Metadata(context.Background(), &proto.Metadata_Request{})
	require.NoError(t, err)
}
