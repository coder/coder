package proto

import (
	"github.com/google/uuid"
)

type ConnectionReporter interface {
	Connect(ConnectEvent) DisconnectionReporter
}

type DisconnectionReporter interface {
	Disconnect(DisconnectEvent)
}

type NoopConnectionReporter struct{}

func (*NoopConnectionReporter) Connect(ConnectEvent) DisconnectionReporter {
	return &NoopDisconnectionReporter{}
}

type NoopDisconnectionReporter struct{}

func (*NoopDisconnectionReporter) Disconnect(DisconnectEvent) {}

// ConnectEvent describes a connection. Its disconnect report repeats every
// field.
type ConnectEvent struct {
	ID uuid.UUID
	// Method is set by the handler, never from client input.
	Method Connection_Method
	// AppName is client-supplied, and empty when unknown.
	AppName         string
	IP              string
	ClientSessionID string
}

type DisconnectEvent struct {
	Code   int
	Reason string
}
