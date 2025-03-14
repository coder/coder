package dbtestutil

import (
	"fmt"
	"errors"
	"context"

	"database/sql/driver"
	"github.com/lib/pq"
	"github.com/coder/coder/v2/coderd/database"

)
var _ database.DialerConnector = &Connector{}
type Connector struct {

	name   string
	driver *Driver

	dialer pq.Dialer
}
func (c *Connector) Connect(_ context.Context) (driver.Conn, error) {
	if c.dialer != nil {
		conn, err := pq.DialOpen(c.dialer, c.name)
		if err != nil {

			return nil, fmt.Errorf("failed to dial open connection: %w", err)
		}
		c.driver.Connections <- conn
		return conn, nil
	}
	conn, err := pq.Driver{}.Open(c.name)
	if err != nil {

		return nil, fmt.Errorf("failed to open connection: %w", err)
	}

	c.driver.Connections <- conn
	return conn, nil
}

func (c *Connector) Driver() driver.Driver {
	return c.driver
}
func (c *Connector) Dialer(dialer pq.Dialer) {
	c.dialer = dialer

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
