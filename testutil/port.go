package testutil

import (
	"math/rand"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
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

// RandomPortNoListen returns a random port in the ephemeral port range.
// Does not attempt to listen and close to find a port as the OS may
// reallocate the port very quickly.
func RandomPortNoListen() uint16 {
	const (
		// Overlap of windows, linux in https://en.wikipedia.org/wiki/Ephemeral_port
		min = 49152
		max = 60999
	)
	n := max - min
	x := rand.Intn(n) //nolint: gosec
	return uint16(min + x)
}
