package database

import (
	"context"
	"database/sql/driver"

	"github.com/lib/pq"
)

// ConnectorCreator can create a driver.Connector.
type ConnectorCreator interface {
	Connector(name string) (driver.Connector, error)
}

// DialerConnector can create a driver.Connector and set a pq.Dialer.
type DialerConnector interface {
	Connect(context.Context) (driver.Conn, error)
	Dialer(dialer pq.Dialer)
}
