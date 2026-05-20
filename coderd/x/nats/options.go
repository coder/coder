package nats

import (
	"time"
)

// PendingLimits configures per-subscription NATS pending limits.
// These limits are applied to each *natsgo.Subscription created on
// the wrapper's subscriber connections via SetPendingLimits. They
// bound each subscription's client-side pending queue independently,
// so one slow listener gets nats.ErrSlowConsumer for its own
// subscription without disrupting other subscriptions multiplexed on
// the same connection.
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

	// MaxPayload is the NATS max payload. Zero means server default.
	MaxPayload int32

	// MaxPending is the per-client outbound pending byte budget enforced
	// by the embedded server. When a client's outbound queue exceeds
	// this, the server treats the client as a slow consumer and
	// disconnects it. Because the wrapper multiplexes many subscriptions
	// on each subscriber connection, this budget bounds the burst
	// headroom for wide local fan-out on any one subscriber conn. Zero
	// means DefaultMaxPending (1 GiB), well above the nats-server
	// default of 64 MiB. Negative means use the server default.
	MaxPending int64

	// DrainTimeout bounds subscription and connection drains in Close.
	// Zero means 30 seconds, matching the NATS Go client default.
	DrainTimeout time.Duration

	// PendingLimits configures per-subscription NATS pending limits.
	// If both Msgs and Bytes are zero, New defaults to
	// {Msgs: -1, Bytes: 512 MiB} (unlimited message count, 512 MiB byte
	// cap) so wide fan-out workloads don't trip the NATS client default
	// limits. Setting either field opts out of the default.
	PendingLimits PendingLimits

	// ReadyTimeout bounds embedded server startup. Zero means
	// DefaultReadyTimeout.
	ReadyTimeout time.Duration

	// ReconnectWait controls client reconnect delay. Zero keeps NATS
	// default.
	ReconnectWait time.Duration

	// InProcess, when true, causes New to construct its publisher and
	// subscriber connections via nats.InProcessServer instead of
	// dialing the embedded server's TCP loopback listener. This skips
	// the kernel socket layer and is intended for benchmarks and
	// tests that want to measure the in-process path. Default false
	// (TCP loopback).
	InProcess bool

	// PublishConns sets the number of TCP-loopback publisher
	// connections New opens to the embedded server. Each Publish call
	// is dispatched to one of these connections selected by a stable
	// hash of the subject, so same-subject publishes are routed to the
	// same connection and preserve per-subject ordering. Multiple
	// publish connections reduce contention on the per-conn write
	// mutex and socket under concurrent publishers across distinct
	// subjects. Zero or negative means 1 (single publish connection),
	// matching the historical behavior. Ignored by NewFromConn, which
	// reuses the externally supplied connection.
	PublishConns int

	// SubscribeConns sets the number of TCP-loopback subscriber
	// connections New opens to the embedded server. Each underlying
	// shared *natsgo.Subscription is assigned to one of these
	// connections by a stable hash of its subject, so all local
	// subscribers for a subject coalesce onto the same shared NATS
	// subscription on the same connection. Multiple subscriber
	// connections spread distinct subjects across multiple TCP
	// read/parser loops and per-conn server-side pending budgets,
	// which is the main subscriber-side bottleneck beyond same-
	// subject coalescing. Zero or negative means 1 (single subscriber
	// connection), matching the historical behavior. Ignored by
	// NewFromConn, which reuses the externally supplied connection.
	SubscribeConns int

	// WriteBufferSize sets the NATS Go client write buffer size, in
	// bytes, applied to every wrapper-owned client connection (all
	// publish conns and all subscriber conns). It maps to
	// natsgo.WriteBufferSize, which controls the flush threshold for
	// the per-connection outbound buffer; nats.go auto-flushes when
	// the buffer fills, and the default is 32 KiB. Larger values
	// amortize syscall and lock overhead at the cost of bursty
	// in-flight bytes, which matters most for 8 KiB+ payloads.
	//
	// Zero preserves the nats.go default (32 KiB). Positive values
	// override it. NewFromConn does not apply this option: it reuses
	// a caller-supplied external *natsgo.Conn whose write buffer is
	// already fixed by whoever opened it.
	WriteBufferSize int
}

// Default values for Options.
const (
	DefaultSubjectPrefix = "coder.v1"
	DefaultReadyTimeout  = 10 * time.Second
	// DefaultMaxPending is the per-client outbound pending byte budget
	// applied to the embedded server. Raised from the nats-server
	// default of 64 MiB to 1 GiB so wide local fan-out on the shared
	// subConn does not trip the server slow-consumer threshold on
	// realistic bursts.
	DefaultMaxPending int64 = 1 << 30
)
