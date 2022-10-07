package agent

import (
	"context"
	"io"
	"net"
	"sync/atomic"

	"cdr.dev/slog"
	"github.com/coder/coder/codersdk"
)

// statsConn wraps a net.Conn with statistics.
type statsConn struct {
	*Stats
	net.Conn `json:"-"`
}

var _ net.Conn = new(statsConn)

func (c *statsConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	atomic.AddInt64(&c.RxBytes, int64(n))
	return n, err
}

func (c *statsConn) Write(b []byte) (n int, err error) {
	n, err = c.Conn.Write(b)
	atomic.AddInt64(&c.TxBytes, int64(n))
	return n, err
}

var _ net.Conn = new(statsConn)

// Stats records the Agent's network connection statistics for use in
// user-facing metrics and debugging.
// Each member value must be written and read with atomic.
type Stats struct {
	NumConns int64 `json:"num_comms"`
	RxBytes  int64 `json:"rx_bytes"`
	TxBytes  int64 `json:"tx_bytes"`
}

func (s *Stats) Copy() *codersdk.AgentStats {
	return &codersdk.AgentStats{
		NumConns: atomic.LoadInt64(&s.NumConns),
		RxBytes:  atomic.LoadInt64(&s.RxBytes),
		TxBytes:  atomic.LoadInt64(&s.TxBytes),
	}
}

// wrapConn returns a new connection that records statistics.
func (s *Stats) wrapConn(conn net.Conn) net.Conn {
	atomic.AddInt64(&s.NumConns, 1)
	cs := &statsConn{
		Stats: s,
		Conn:  conn,
	}

	return cs
}

// StatsReporter periodically accept and records agent stats.
type StatsReporter func(
	ctx context.Context,
	log slog.Logger,
	stats func() *codersdk.AgentStats,
) (io.Closer, error)
