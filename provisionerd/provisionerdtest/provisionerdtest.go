package provisionerdtest

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"storj.io/drpc/drpcconn"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
	"github.com/coder/coder/provisioner/terraform"
	"github.com/coder/coder/provisionerd"
	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

// New creates a provisionerd instance with provisioners registered.
func New(t *testing.T, client *codersdk.Client) io.Closer {
	tfClient, tfServer := provisionersdk.TransportPipe()
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(func() {
		_ = tfClient.Close()
		_ = tfServer.Close()
		cancelFunc()
	})
	go func() {
		err := terraform.Serve(ctx, &terraform.ServeOptions{
			ServeOptions: &provisionersdk.ServeOptions{
				Transport: tfServer,
			},
		})
		require.NoError(t, err)
	}()

	return provisionerd.New(client, &provisionerd.Options{
		Logger:       slogtest.Make(t, nil).Named("provisionerd").Leveled(slog.LevelDebug),
		PollInterval: 50 * time.Millisecond,
		Provisioners: provisionerd.Provisioners{
			database.ProvisionerTypeTerraform: proto.NewDRPCProvisionerClient(drpcconn.New(tfClient)),
		},
		WorkDirectory: t.TempDir(),
	})
}
