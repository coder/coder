package derpmesh_test

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"tailscale.com/derp"
	"tailscale.com/derp/derphttp"
	"tailscale.com/types/key"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/enterprise/highavailability/derpmesh"
	"github.com/coder/coder/tailnet"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestDERPMesh(t *testing.T) {
	t.Parallel()
	t.Run("ExchangeMessages", func(t *testing.T) {
		// This tests messages passing through multiple DERP servers.
		t.Parallel()
		firstServer, firstServerURL := startDERP(t)
		defer firstServer.Close()
		secondServer, secondServerURL := startDERP(t)
		firstMesh := derpmesh.New(slogtest.Make(t, nil).Named("first").Leveled(slog.LevelDebug), firstServer)
		firstMesh.SetAddresses([]string{secondServerURL})
		secondMesh := derpmesh.New(slogtest.Make(t, nil).Named("second").Leveled(slog.LevelDebug), secondServer)
		secondMesh.SetAddresses([]string{firstServerURL})
		defer firstMesh.Close()
		defer secondMesh.Close()

		first := key.NewNode()
		second := key.NewNode()
		firstClient, err := derphttp.NewClient(first, secondServerURL, tailnet.Logger(slogtest.Make(t, nil)))
		require.NoError(t, err)
		secondClient, err := derphttp.NewClient(second, firstServerURL, tailnet.Logger(slogtest.Make(t, nil)))
		require.NoError(t, err)
		err = secondClient.Connect(context.Background())
		require.NoError(t, err)

		sent := []byte("hello world")
		err = firstClient.Send(second.Public(), sent)
		require.NoError(t, err)

		got := recvData(t, secondClient)
		require.Equal(t, sent, got)
	})
	t.Run("RemoveAddress", func(t *testing.T) {
		// This tests messages passing through multiple DERP servers.
		t.Parallel()
		server, serverURL := startDERP(t)
		mesh := derpmesh.New(slogtest.Make(t, nil).Named("first").Leveled(slog.LevelDebug), server)
		mesh.SetAddresses([]string{"http://fake.com"})
		// This should trigger a removal...
		mesh.SetAddresses([]string{})
		defer mesh.Close()

		first := key.NewNode()
		second := key.NewNode()
		firstClient, err := derphttp.NewClient(first, serverURL, tailnet.Logger(slogtest.Make(t, nil)))
		require.NoError(t, err)
		secondClient, err := derphttp.NewClient(second, serverURL, tailnet.Logger(slogtest.Make(t, nil)))
		require.NoError(t, err)
		err = secondClient.Connect(context.Background())
		require.NoError(t, err)
		sent := []byte("hello world")
		err = firstClient.Send(second.Public(), sent)
		require.NoError(t, err)
		got := recvData(t, secondClient)
		require.Equal(t, sent, got)
	})
	t.Run("TwentyMeshes", func(t *testing.T) {
		t.Parallel()
		meshes := make([]*derpmesh.Mesh, 0, 20)
		serverURLs := make([]string, 0, 20)
		for i := 0; i < 20; i++ {
			server, url := startDERP(t)
			mesh := derpmesh.New(slogtest.Make(t, nil).Named("mesh").Leveled(slog.LevelDebug), server)
			t.Cleanup(func() {
				_ = server.Close()
				_ = mesh.Close()
			})
			serverURLs = append(serverURLs, url)
			meshes = append(meshes, mesh)
		}
		for _, mesh := range meshes {
			mesh.SetAddresses(serverURLs)
		}

		first := key.NewNode()
		second := key.NewNode()
		firstClient, err := derphttp.NewClient(first, serverURLs[9], tailnet.Logger(slogtest.Make(t, nil)))
		require.NoError(t, err)
		secondClient, err := derphttp.NewClient(second, serverURLs[16], tailnet.Logger(slogtest.Make(t, nil)))
		require.NoError(t, err)
		err = secondClient.Connect(context.Background())
		require.NoError(t, err)

		sent := []byte("hello world")
		err = firstClient.Send(second.Public(), sent)
		require.NoError(t, err)

		got := recvData(t, secondClient)
		require.Equal(t, sent, got)
	})
}

func recvData(t *testing.T, client *derphttp.Client) []byte {
	for {
		msg, err := client.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		assert.NoError(t, err)
		t.Logf("derp: %T", msg)
		switch msg := msg.(type) {
		case derp.ReceivedPacket:
			return msg.Data
		default:
			// Drop all others!
		}
	}
}

func startDERP(t *testing.T) (*derp.Server, string) {
	logf := tailnet.Logger(slogtest.Make(t, nil))
	d := derp.NewServer(key.NewNode(), logf)
	d.SetMeshKey("some-key")
	server := httptest.NewUnstartedServer(derphttp.Handler(d))
	server.Start()
	t.Cleanup(func() {
		_ = d.Close()
	})
	t.Cleanup(server.Close)
	return d, server.URL
}
