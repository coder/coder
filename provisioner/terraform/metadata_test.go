//go:build linux

package terraform

import (
	"context"
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/releases"
	"github.com/stretchr/testify/require"
	"storj.io/drpc/drpcconn"

	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestMetadata(t *testing.T) {
	t.Parallel()

	installer := &releases.ExactVersion{
		Product: product.Terraform,
		Version: version.Must(version.NewVersion("1.1.2")),
	}
	execPath, err := installer.Install(context.Background())
	require.NoError(t, err)

	client, server := provisionersdk.TransportPipe()
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
		cancelFunc()
	})
	go func() {
		err := Serve(ctx, &ServeOptions{
			BinaryPath: execPath,
			ServeOptions: &provisionersdk.ServeOptions{
				Transport: server,
			},
		})
		require.NoError(t, err)
	}()
	api := proto.NewDRPCProvisionerClient(drpcconn.New(client))
	_, err = api.Metadata(ctx, &proto.Metadata_Request{})
	require.NoError(t, err)
}
