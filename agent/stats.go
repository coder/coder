package agent

import (
	"net"
	"time"
)

// ConnStats wraps a net.Conn with statistics.
type ConnStats struct {
	CreatedAt time.Time `json:"created_at,omitempty"`
	Protocol  string    `json:"protocol,omitempty"`
	RxBytes   uint64    `json:"rx_bytes,omitempty"`
	TxBytes   uint64    `json:"tx_bytes,omitempty"`

	net.Conn `json:"-"`
}

var _ net.Conn = new(ConnStats)

func (c *ConnStats) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	c.RxBytes += uint64(n)
	return n, err
}

func (c *ConnStats) Write(b []byte) (n int, err error) {
	n, err = c.Conn.Write(b)
	c.TxBytes += uint64(n)
	return n, err
}

var _ net.Conn = new(ConnStats)

// Stats records the Agent's network connection statistics for use in
// user-facing metrics and debugging.
type Stats struct {
	Conns []ConnStats `json:"conns,omitempty"`
}
