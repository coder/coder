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
	Method          Connection_Method
	AppName         string
	IP              string
	ClientSessionID string
}

type DisconnectEvent struct {
	Code    int
	Reason  string
	RxBytes int64
	TxBytes int64
}
