package dbtestutil

import (
	"context"
	"database/sql/driver"

	"github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

var _ database.DialerConnector = &Connector{}

type Connector struct {
	name   string
	driver *Driver
	// Note: pgx handles dialing differently via config
}

func (c *Connector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := stdlib.GetDefaultDriver().Open(c.name)
	if err != nil {
		return nil, xerrors.Errorf("failed to open connection: %w", err)
	}

	c.driver.Connections <- conn

	return conn, nil
}

func (c *Connector) Driver() driver.Driver {
	return c.driver
}

func (c *Connector) Dialer(dialer interface{}) {
	// Note: pgx handles dialing differently via config
	// This method is kept for interface compatibility but is a no-op
}

type Driver struct {
	Connections chan driver.Conn
}

func NewDriver() *Driver {
	return &Driver{
		Connections: make(chan driver.Conn, 1),
	}
}

func (d *Driver) Connector(name string) (driver.Connector, error) {
	return &Connector{
		name:   name,
		driver: d,
	}, nil
}

func (d *Driver) Open(name string) (driver.Conn, error) {
	c, err := d.Connector(name)
	if err != nil {
		return nil, err
	}

	return c.Connect(context.Background())
}

func (d *Driver) Close() {
	close(d.Connections)
}
