//go:build linux || (windows && amd64)

package agent

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOSListeningPortsGetter(t *testing.T) {
	t.Parallel()

	uut := &osListeningPortsGetter{
		cacheDuration: 1 * time.Hour,
	}

	l, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	defer l.Close()

	ports, err := uut.GetListeningPorts()
	require.NoError(t, err)
	found := false
	for _, port := range ports {
		// #nosec G115 - Safe conversion as TCP port numbers are within uint16 range (0-65535)
		if port.Port == uint16(l.Addr().(*net.TCPAddr).Port) {
			found = true
			break
		}
	}
	require.True(t, found)

	// check that we cache the ports
	err = l.Close()
	require.NoError(t, err)
	portsNew, err := uut.GetListeningPorts()
	require.NoError(t, err)
	require.Equal(t, ports, portsNew)

	// note that it's unsafe to try to assert that a port does not exist in the response
	// because the OS may reallocate the port very quickly.
}
