package agent_test

import (
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/agent"
)

func TestConnStats(t *testing.T) {
	t.Parallel()

	t.Run("Write", func(t *testing.T) {
		t.Parallel()

		c1, c2 := net.Pipe()

		payload := []byte("dogs & cats")
		statsConn := &agent.ConnStats{Conn: c1}

		got := make(chan []byte)
		go func() {
			b, _ := io.ReadAll(c2)
			got <- b
		}()
		n, err := statsConn.Write(payload)
		require.NoError(t, err)
		assert.Equal(t, len(payload), n)
		statsConn.Close()

		require.Equal(t, payload, <-got)

		require.EqualValues(t, statsConn.TxBytes, len(payload))
		require.EqualValues(t, statsConn.RxBytes, 0)
	})

	t.Run("Read", func(t *testing.T) {
		t.Parallel()

		c1, c2 := net.Pipe()

		payload := []byte("cats & dogs")
		statsConn := &agent.ConnStats{Conn: c1}

		go func() {
			c2.Write(payload)
			c2.Close()
		}()

		got, err := io.ReadAll(statsConn)
		require.NoError(t, err)
		assert.Equal(t, len(payload), len(got))

		require.EqualValues(t, statsConn.RxBytes, len(payload))
		require.EqualValues(t, statsConn.TxBytes, 0)
	})
}
