package database

import (
	"context"
	"database/sql/driver"

	"github.com/lib/pq"
)

type ConnectorCreator interface {
	Connector(name string) (driver.Connector, error)
}

type DialerConnector interface {
	Connect(context.Context) (driver.Conn, error)
	Dialer(dialer pq.Dialer)
}
