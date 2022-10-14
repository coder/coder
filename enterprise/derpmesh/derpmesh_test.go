package derpmesh_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"tailscale.com/derp"
	"tailscale.com/derp/derphttp"
	"tailscale.com/types/key"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/enterprise/derpmesh"
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
		firstServer, firstServerURL, firstTLSName := startDERP(t)
		defer firstServer.Close()
		secondServer, secondServerURL, secondTLSName := startDERP(t)
		firstMesh := derpmesh.New(slogtest.Make(t, nil).Named("first").Leveled(slog.LevelDebug), firstServer, firstTLSName)
		firstMesh.SetAddresses([]string{secondServerURL})
		secondMesh := derpmesh.New(slogtest.Make(t, nil).Named("second").Leveled(slog.LevelDebug), secondServer, secondTLSName)
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
	t.Run("ExchangeMessages", func(t *testing.T) {
		// This tests messages passing through multiple DERP servers.
		t.Parallel()
		firstServer, firstServerURL, firstTLSName := startDERP(t)
		defer firstServer.Close()
		secondServer, secondServerURL, secondTLSName := startDERP(t)
		firstMesh := derpmesh.New(slogtest.Make(t, nil).Named("first").Leveled(slog.LevelDebug), firstServer, firstTLSName)
		firstMesh.SetAddresses([]string{secondServerURL})
		secondMesh := derpmesh.New(slogtest.Make(t, nil).Named("second").Leveled(slog.LevelDebug), secondServer, secondTLSName)
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
		server, serverURL, tlsName := startDERP(t)
		mesh := derpmesh.New(slogtest.Make(t, nil).Named("first").Leveled(slog.LevelDebug), server, tlsName)
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
			server, url, tlsName := startDERP(t)
			mesh := derpmesh.New(slogtest.Make(t, nil).Named("mesh").Leveled(slog.LevelDebug), server, tlsName)
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

func startDERP(t *testing.T) (*derp.Server, string, *tls.Config) {
	logf := tailnet.Logger(slogtest.Make(t, nil))
	d := derp.NewServer(key.NewNode(), logf)
	d.SetMeshKey("some-key")
	server := httptest.NewUnstartedServer(derphttp.Handler(d))
	commonName := "something.org"
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{generateTLSCertificate(t, commonName)},
	}
	server.Start()
	t.Cleanup(func() {
		_ = d.Close()
	})
	t.Cleanup(server.Close)
	return d, server.URL, server.TLS
}

func generateTLSCertificate(t testing.TB, commonName string) tls.Certificate {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
			CommonName:   commonName,
		},
		DNSNames:  []string{commonName},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour * 24 * 180),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)
	var certFile bytes.Buffer
	require.NoError(t, err)
	_, err = certFile.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes}))
	require.NoError(t, err)
	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	var keyFile bytes.Buffer
	err = pem.Encode(&keyFile, &pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyBytes})
	require.NoError(t, err)
	cert, err := tls.X509KeyPair(certFile.Bytes(), keyFile.Bytes())
	require.NoError(t, err)
	return cert
}
