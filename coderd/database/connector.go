package database

import (
	"database/sql/driver"
)

// ConnectorCreator is a driver.Driver that can create a driver.Connector.
type ConnectorCreator interface {
	driver.Driver
	Connector(name string) (driver.Connector, error)
}

// DialerConnector is a driver.Connector that can set a dialer.
// Note: pgx uses a different approach for custom dialers via config
type DialerConnector interface {
	driver.Connector
	// Dialer functionality is handled differently in pgx
	// Use stdlib.RegisterConnConfig for custom connection configuration
}
