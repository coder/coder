//nolint:testpackage // Exercises internal pool hashing.
package nats

import (
	"testing"

	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

func TestPickConn_DifferentSubjectsUseDifferentConns(t *testing.T) {
	t.Parallel()
	var a, b natsgo.Conn
	pool := []*natsgo.Conn{&a, &b}

	require.NotSame(t, pickConn(pool, "a"), pickConn(pool, "b"))
}
