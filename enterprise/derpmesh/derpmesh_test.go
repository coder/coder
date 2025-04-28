package derpmesh_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"tailscale.com/derp"
	"tailscale.com/derp/derphttp"
	"tailscale.com/types/key"

	"github.com/coder/coder/v2/enterprise/derpmesh"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

func TestDERPMesh(t *testing.T) {
	t.Parallel()
	commonName := "something.org"
	rawCert := testutil.GenerateTLSCertificate(t, commonName)
	certificate, err := x509.ParseCertificate(rawCert.Certificate[0])
	require.NoError(t, err)
	pool := x509.NewCertPool()
	pool.AddCert(certificate)
	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		ServerName:   commonName,
		RootCAs:      pool,
		Certificates: []tls.Certificate{rawCert},
	}

	t.Run("ExchangeMessages", func(t *testing.T) {
		// This tests messages passing through multiple DERP servers.
		t.Parallel()
		firstServer, firstServerURL := startDERP(t, tlsConfig)
		defer firstServer.Close()
		secondServer, secondServerURL := startDERP(t, tlsConfig)
		firstMesh := derpmesh.New(testutil.Logger(t).Named("first"), firstServer, tlsConfig)
		firstMesh.SetAddresses([]string{secondServerURL}, false)
		secondMesh := derpmesh.New(testutil.Logger(t).Named("second"), secondServer, tlsConfig)
		secondMesh.SetAddresses([]string{firstServerURL}, false)
		defer firstMesh.Close()
		defer secondMesh.Close()

		first := key.NewNode()
		second := key.NewNode()
		firstClient, err := derphttp.NewClient(first, secondServerURL, tailnet.Logger(testutil.Logger(t)))
		require.NoError(t, err)
		firstClient.TLSConfig = tlsConfig
		secondClient, err := derphttp.NewClient(second, firstServerURL, tailnet.Logger(testutil.Logger(t)))
		require.NoError(t, err)
		secondClient.TLSConfig = tlsConfig
		err = secondClient.Connect(context.Background())
		require.NoError(t, err)

		closed := make(chan struct{})
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		sent := []byte("hello world")
		go func() {
			defer close(closed)
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
				err = firstClient.Send(second.Public(), sent)
				require.NoError(t, err)
			}
		}()

		got := recvData(t, secondClient)
		require.Equal(t, sent, got)
		cancelFunc()
		<-closed
	})
	t.Run("RemoveAddress", func(t *testing.T) {
		// This tests messages passing through multiple DERP servers.
		t.Parallel()
		server, serverURL := startDERP(t, tlsConfig)
		mesh := derpmesh.New(testutil.Logger(t).Named("first"), server, tlsConfig)
		mesh.SetAddresses([]string{"http://fake.com"}, false)
		// This should trigger a removal...
		mesh.SetAddresses([]string{}, false)
		defer mesh.Close()

		first := key.NewNode()
		second := key.NewNode()
		firstClient, err := derphttp.NewClient(first, serverURL, tailnet.Logger(testutil.Logger(t)))
		require.NoError(t, err)
		firstClient.TLSConfig = tlsConfig
		secondClient, err := derphttp.NewClient(second, serverURL, tailnet.Logger(testutil.Logger(t)))
		require.NoError(t, err)
		secondClient.TLSConfig = tlsConfig
		err = secondClient.Connect(context.Background())
		require.NoError(t, err)

		closed := make(chan struct{})
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		sent := []byte("hello world")
		go func() {
			defer close(closed)
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
				err = firstClient.Send(second.Public(), sent)
				require.NoError(t, err)
			}
		}()
		got := recvData(t, secondClient)
		require.Equal(t, sent, got)
		cancelFunc()
		<-closed
	})
	t.Run("TwentyMeshes", func(t *testing.T) {
		t.Parallel()
		meshes := make([]*derpmesh.Mesh, 0, 20)
		serverURLs := make([]string, 0, 20)
		for i := 0; i < 20; i++ {
			server, url := startDERP(t, tlsConfig)
			mesh := derpmesh.New(testutil.Logger(t).Named("mesh"), server, tlsConfig)
			t.Cleanup(func() {
				_ = server.Close()
				_ = mesh.Close()
			})
			serverURLs = append(serverURLs, url)
			meshes = append(meshes, mesh)
		}
		for _, mesh := range meshes {
			mesh.SetAddresses(serverURLs, true)
		}

		first := key.NewNode()
		second := key.NewNode()
		firstClient, err := derphttp.NewClient(first, serverURLs[9], tailnet.Logger(testutil.Logger(t)))
		require.NoError(t, err)
		firstClient.TLSConfig = tlsConfig
		secondClient, err := derphttp.NewClient(second, serverURLs[16], tailnet.Logger(testutil.Logger(t)))
		require.NoError(t, err)
		secondClient.TLSConfig = tlsConfig
		err = secondClient.Connect(context.Background())
		require.NoError(t, err)

		closed := make(chan struct{})
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		sent := []byte("hello world")
		go func() {
			defer close(closed)
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
				err = firstClient.Send(second.Public(), sent)
				require.NoError(t, err)
			}
		}()

		got := recvData(t, secondClient)
		require.Equal(t, sent, got)
		cancelFunc()
		<-closed
	})
	t.Run("CycleAddresses", func(t *testing.T) {
		t.Parallel()
		firstServer, firstServerURL := startDERP(t, tlsConfig)
		defer firstServer.Close()
		secondServer, secondServerURL := startDERP(t, tlsConfig)
		firstMesh := derpmesh.New(testutil.Logger(t).Named("first"), firstServer, tlsConfig)
		firstMesh.SetAddresses([]string{secondServerURL}, false)
		secondMesh := derpmesh.New(testutil.Logger(t).Named("second"), secondServer, tlsConfig)
		// Ensures that the client properly re-adds the address after it's removed.
		secondMesh.SetAddresses([]string{firstServerURL}, true)
		secondMesh.SetAddresses([]string{}, true)
		secondMesh.SetAddresses([]string{firstServerURL}, true)
		defer firstMesh.Close()
		defer secondMesh.Close()

		first := key.NewNode()
		second := key.NewNode()
		firstClient, err := derphttp.NewClient(first, secondServerURL, tailnet.Logger(testutil.Logger(t)))
		require.NoError(t, err)
		firstClient.TLSConfig = tlsConfig
		secondClient, err := derphttp.NewClient(second, firstServerURL, tailnet.Logger(testutil.Logger(t)))
		require.NoError(t, err)
		secondClient.TLSConfig = tlsConfig
		err = secondClient.Connect(context.Background())
		require.NoError(t, err)

		closed := make(chan struct{})
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		sent := []byte("hello world")
		go func() {
			defer close(closed)
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
				err = firstClient.Send(second.Public(), sent)
				require.NoError(t, err)
			}
		}()

		got := recvData(t, secondClient)
		require.Equal(t, sent, got)
		cancelFunc()
		<-closed
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

func startDERP(t *testing.T, tlsConfig *tls.Config) (*derp.Server, string) {
	logf := tailnet.Logger(testutil.Logger(t))
	d := derp.NewServer(key.NewNode(), logf)
	d.SetMeshKey("some-key")
	server := httptest.NewUnstartedServer(derphttp.Handler(d))
	server.TLS = tlsConfig
	server.StartTLS()
	t.Cleanup(func() {
		_ = d.Close()
	})
	t.Cleanup(server.Close)
	return d, server.URL
}
