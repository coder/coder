package database

import (
	"database/sql/driver"

	"github.com/lib/pq"
)

// ConnectorCreator is a driver.Driver that can create a driver.Connector.
type ConnectorCreator interface {
	driver.Driver
	Connector(name string) (driver.Connector, error)
}

// DialerConnector is a driver.Connector that can set a pq.Dialer.
type DialerConnector interface {
	driver.Connector
	Dialer(dialer pq.Dialer)
}
