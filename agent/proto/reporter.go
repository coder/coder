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

type ConnectEvent struct {
	ID              uuid.UUID
	Type            Connection_Type
	AppName         string
	IP              string
	ClientSessionID string
	// Path, if set, reports the current network path of the connection. It
	// is called when the connection is reported and again when the
	// disconnection is reported.
	Path func() Connection_Path
}

type DisconnectEvent struct {
	Code   int
	Reason string
}
