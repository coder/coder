package testutil

import (
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	// Overlap of windows, linux in https://en.wikipedia.org/wiki/Ephemeral_port
	minPort = 49152
	maxPort = 60999
)

var (
	rndMu   sync.Mutex
	rndPort = maxPort
)

// RandomPort is a helper function to find a free random port.
// Note that the OS may reallocate the port very quickly, so
// this is not _guaranteed_.
func RandomPort(t *testing.T) int {
	random, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "failed to listen on localhost")
	_ = random.Close()
	tcpAddr, valid := random.Addr().(*net.TCPAddr)
	require.True(t, valid, "random port address is not a *net.TCPAddr?!")
	return tcpAddr.Port
}

// EphemeralPortNoListen returns the next port in the ephemeral port range.
// Does not attempt to listen and close to find a port as the OS may
// reallocate the port very quickly.
func EphemeralPortNoListen(*testing.T) uint16 {
	rndMu.Lock()
	p := rndPort

	rndPort--
	if rndPort < minPort {
		rndPort = maxPort
	}
	rndMu.Unlock()
	return uint16(p)
}
