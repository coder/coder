package testutil

import (
	"math/rand"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var (
	// nolint:gosec // not used for cryptography
	rnd   = rand.New(rand.NewSource(time.Now().Unix()))
	rndMu sync.Mutex
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
func RandomPortNoListen(*testing.T) uint16 {
	const (
		// Overlap of windows, linux in https://en.wikipedia.org/wiki/Ephemeral_port
		minPort = 49152
		maxPort = 60999
	)
	n := maxPort - minPort
	rndMu.Lock()
	x := rnd.Intn(n)
	rndMu.Unlock()
	// #nosec G115 - Safe conversion since minPort and x are explicitly within the uint16 range
	return uint16(minPort + x)
}
