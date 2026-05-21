package nats

import (
	"time"
)

// PendingLimits configures per-subscription NATS pending limits set
// via SetPendingLimits on each *natsgo.Subscription. Limits are
// per-subscription so one slow listener cannot disrupt others
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
	// ServerName is the NATS server name. If empty, New derives one.
	ServerName string

	// ClientName is the NATS client name. If empty, New derives one.
	ClientName string

	// MaxPayload is the NATS max payload. Zero means server default.
	MaxPayload int32

	// MaxPending is the per-client outbound pending byte budget on the
	// embedded server. Zero means DefaultMaxPending; negative means use
	// the nats-server default (64 MiB).
	MaxPending int64

	// DrainTimeout bounds subscription and connection drains in Close.
	// Zero means 30 seconds, matching the NATS Go client default.
	DrainTimeout time.Duration

	// PendingLimits configures per-subscription NATS pending limits.
	// If both fields are zero, New defaults to {Msgs: -1, Bytes: 512 MiB}
	// so wide fan-out workloads don't trip the NATS client defaults.
	PendingLimits PendingLimits

	// ReadyTimeout bounds embedded server startup. Zero means
	// DefaultReadyTimeout.
	ReadyTimeout time.Duration

	// ReconnectWait controls client reconnect delay. Zero keeps the
	// NATS default.
	ReconnectWait time.Duration

	// InProcess, when true, uses nats.InProcessServer instead of TCP
	// loopback for publisher and subscriber connections. Intended for
	// benchmarks and tests. Default false (TCP loopback).
	InProcess bool

	// PublishConns is the number of publisher connections. Each Publish
	// is routed by a stable hash of the subject so same-subject
	// publishes preserve per-subject ordering. Zero or negative means 1.
	PublishConns int

	// SubscribeConns is the number of subscriber connections. Each
	// shared subscription is pinned to one connection by a stable hash
	// of its subject. Zero or negative means 1.
	SubscribeConns int
}

// Default values for Options.
const (
	DefaultReadyTimeout = 10 * time.Second
	// DefaultMaxPending is the per-client outbound pending byte budget
	// (1 GiB), raised from the nats-server default of 64 MiB so wide
	// local fan-out does not trip the server slow-consumer threshold.
	DefaultMaxPending int64 = 1 << 30
)
