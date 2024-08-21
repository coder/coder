package dbtestutil

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"

	"github.com/lib/pq"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/cryptorand"
)

var (
	_ driver.Driver             = &Driver{}
	_ database.ConnectorCreator = &Driver{}
	_ database.DialerConnector  = &Connector{}
)

type Driver struct {
	name        string
	inner       driver.Driver
	connections []driver.Conn
	listeners   map[chan struct{}]chan struct{}
}

func Register() (*Driver, error) {
	db, err := sql.Open("postgres", "")
	if err != nil {
		return nil, xerrors.Errorf("failed to open database: %w", err)
	}

	su, err := cryptorand.StringCharset(cryptorand.Alpha, 10)
	if err != nil {
		return nil, xerrors.Errorf("failed to generate random string: %w", err)
	}

	d := &Driver{
		name:      fmt.Sprintf("postgres-test-%s", su),
		inner:     db.Driver(),
		listeners: make(map[chan struct{}]chan struct{}),
	}

	sql.Register(d.name, d)

	return d, nil
}

func (d *Driver) Open(name string) (driver.Conn, error) {
	conn, err := d.inner.Open(name)
	if err != nil {
		return nil, xerrors.Errorf("failed to open connection: %w", err)
	}

	d.AddConnection(conn)

	return conn, nil
}

func (d *Driver) Connector(name string) (driver.Connector, error) {
	return &Connector{
		name:   name,
		driver: d,
	}, nil
}

func (d *Driver) Name() string {
	return d.name
}

func (d *Driver) AddConnection(conn driver.Conn) {
	d.connections = append(d.connections, conn)
	for listener := range d.listeners {
		d.listeners[listener] <- struct{}{}
	}
}

func (d *Driver) WaitForConnection() {
	ch := make(chan struct{})
	d.listeners[ch] = ch
	<-ch
	delete(d.listeners, ch)
}

func (d *Driver) DropConnections() {
	for _, conn := range d.connections {
		_ = conn.Close()
	}
	d.connections = nil
}

type Connector struct {
	name   string
	driver *Driver
	dialer pq.Dialer
}

func (c *Connector) Connect(_ context.Context) (driver.Conn, error) {
	if c.dialer != nil {
		conn, err := pq.DialOpen(c.dialer, c.name)
		if err != nil {
			return nil, xerrors.Errorf("failed to dial open connection: %w", err)
		}

		c.driver.AddConnection(conn)

		return conn, nil
	}

	conn, err := c.driver.Open(c.name)
	if err != nil {
		return nil, xerrors.Errorf("failed to open connection: %w", err)
	}

	return conn, nil
}

func (c *Connector) Driver() driver.Driver {
	return c.driver
}

func (c *Connector) Dialer(dialer pq.Dialer) {
	c.dialer = dialer
}
