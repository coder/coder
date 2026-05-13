package nats

import (
	"crypto/tls"
	"time"
)

// PendingLimits configures per-subscription NATS pending limits.
// These limits are applied to each *natsgo.Subscription created on the
// wrapper's shared subscriber connection (subConn) via
// SetPendingLimits. They bound each subscription's client-side pending
// queue independently, so one slow listener gets nats.ErrSlowConsumer
// for its own subscription without disrupting other subscriptions
// multiplexed on the same connection.
type PendingLimits struct {
	// Msgs is the per-subscription pending message limit.
	// Zero keeps the NATS client default. Negative disables this limit.
	Msgs int

	// Bytes is the per-subscription pending byte limit.
	// Zero keeps the NATS client default. Negative disables this limit.
	Bytes int
}

// Options configures the embedded NATS Pubsub.
type Options struct {
	// ServerName is the stable NATS server name. If empty, New derives one.
	ServerName string

	// ClientName is the NATS client name used by the embedded pubsub
	// connection. If empty, New derives one from ServerName.
	ClientName string

	// ClusterName is the NATS cluster name. If empty, use DefaultClusterName.
	ClusterName string

	// PeerProvider returns startup cluster peers. Optional; when nil or
	// when it returns zero peers, the embedded server still starts in
	// cluster mode as a "cluster of 1" so peers can be added later via
	// RefreshPeers without restart.
	PeerProvider PeerProvider

	// ClusterToken is the shared secret used for NATS route
	// authentication. Optional; if empty, an ephemeral random token is
	// generated internally at startup. Supply a stable token when this
	// process is intended to interoperate with other replicas.
	ClusterToken string

	// ClusterTLSConfig enables TLS for route connections when non-nil.
	// Nil means plaintext routes protected only by ClusterToken.
	ClusterTLSConfig *tls.Config

	// ClusterHost is the local route listener bind host in cluster mode.
	// If empty, use "127.0.0.1" for tests and non-wired package usage.
	ClusterHost string

	// ClusterPort is the local route listener port in cluster mode.
	// Zero means choose an available port where NATS supports random bind.
	ClusterPort int

	// ClusterAdvertise is the host:port peers should use to reach this
	// server. In integration, set this to the replica route address, not a
	// load balancer.
	ClusterAdvertise string

	// RoutePoolSize is pinned in all replicas. Zero means
	// DefaultRoutePoolSize.
	RoutePoolSize int

	// MaxPayload is the NATS max payload. Zero means server default.
	MaxPayload int32

	// DrainTimeout bounds subscription and connection drains in Close.
	// Zero means 30 seconds, matching the NATS Go client default.
	DrainTimeout time.Duration

	// PendingLimits configures per-subscription NATS pending limits.
	// If both Msgs and Bytes are zero, New defaults to
	// {Msgs: -1, Bytes: 512 MiB} (unlimited message count, 512 MiB byte
	// cap) so wide fan-out workloads don't trip the NATS client default
	// limits. Setting either field opts out of the default.
	PendingLimits PendingLimits

	// ConnectTimeout bounds the initial client connection. Zero means 2
	// seconds.
	ConnectTimeout time.Duration

	// ReadyTimeout bounds embedded server startup. Zero means
	// DefaultReadyTimeout.
	ReadyTimeout time.Duration

	// ReconnectWait controls client reconnect delay. Zero keeps NATS
	// default.
	ReconnectWait time.Duration

	// MaxReconnects controls client reconnect attempts. Zero keeps NATS
	// default. Negative means retry forever, following nats.go semantics.
	MaxReconnects int

	// NoServerLog disables routing embedded server logs into logger.
	NoServerLog bool
}

// Default values for Options.
const (
	DefaultClusterName   = "coder"
	DefaultSubjectPrefix = "coder.v1"
	DefaultRoutePoolSize = 3
	DefaultReadyTimeout  = 10 * time.Second
)
