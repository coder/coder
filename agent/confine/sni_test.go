package confine_test

import (
	"crypto/tls"
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/confine"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

func TestSNIListenerExtractsAndDenies(t *testing.T) {
	t.Parallel()

	engine := confine.NewPolicyEngine("", 0)
	events := make(chan confine.NetworkEvent, 1)
	listener, err := confine.ListenSNI("127.0.0.1:0", engine, func(event confine.NetworkEvent) { events <- event })
	require.NoError(t, err)
	defer listener.Close()

	conn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	//nolint:gosec // The handshake is expected to be denied before certificate verification.
	tlsConn := tls.Client(conn, &tls.Config{ServerName: "Denied.Example.COM.", InsecureSkipVerify: true})
	require.Error(t, tlsConn.Handshake())
	require.NoError(t, conn.Close())

	event := <-events
	require.Equal(t, agentsdk.AISandboxNetworkProtocolSNI, event.Protocol)
	require.Equal(t, agentsdk.AISandboxNetworkEventActionDenied, event.Action)
	require.Equal(t, "denied.example.com", event.Host)
	require.Equal(t, 443, event.Port)
}
