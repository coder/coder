// Package wsconncache caches workspace agent connections by UUID.
package wsconncache

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/atomic"
	"golang.org/x/xerrors"

	"github.com/coder/coder/agent"
)

// New creates a new workspace connection cache that closes
// connections after the inactive timeout provided.
//
// Agent connections are cached due to WebRTC negotiation
// taking a few hundred milliseconds.
func New(dialer Dialer, inactiveTimeout time.Duration) *Cache {
	if inactiveTimeout == 0 {
		inactiveTimeout = 5 * time.Minute
	}
	return &Cache{
		conns:           make(map[uuid.UUID]*Conn),
		dialer:          dialer,
		inactiveTimeout: inactiveTimeout,
	}
}

// Dialer creates a new agent connection by ID.
type Dialer func(r *http.Request, id uuid.UUID) (*agent.Conn, error)

// Conn wraps an agent connection with a reusable HTTP transport.
type Conn struct {
	*agent.Conn

	locks         atomic.Uint64
	timeoutMutex  sync.Mutex
	timeout       *time.Timer
	timeoutCancel context.CancelFunc
	transport     *http.Transport
}

func (c *Conn) HTTPTransport() *http.Transport {
	return c.transport
}

// Close ends the HTTP transport if exists, and closes the agent.
func (c *Conn) Close() error {
	if c.transport != nil {
		c.transport.CloseIdleConnections()
	}
	if c.timeout != nil {
		c.timeout.Stop()
	}
	return c.Conn.Close()
}

type Cache struct {
	connMutex       sync.RWMutex
	conns           map[uuid.UUID]*Conn
	dialer          Dialer
	inactiveTimeout time.Duration
}

func (c *Cache) Acquire(r *http.Request, id uuid.UUID) (*Conn, func(), error) {
	c.connMutex.RLock()
	conn, exists := c.conns[id]
	c.connMutex.RUnlock()
	if !exists {
		agentConn, err := c.dialer(r, id)
		if err != nil {
			return nil, nil, xerrors.Errorf("dial: %w", err)
		}
		timeoutCtx, timeoutCancelFunc := context.WithCancel(context.Background())
		defaultTransport, valid := http.DefaultTransport.(*http.Transport)
		if !valid {
			panic("dev error: default transport is the wrong type")
		}
		transport := defaultTransport.Clone()
		transport.DialContext = agentConn.DialContext
		conn = &Conn{
			Conn:          agentConn,
			timeoutCancel: timeoutCancelFunc,
			transport:     transport,
		}
		go func() {
			select {
			case <-timeoutCtx.Done():
			case <-conn.Closed():
			}
			c.connMutex.Lock()
			delete(c.conns, id)
			c.connMutex.Unlock()
			// This should close after the delete so callers
			// can check the `Closed()` channel for this to be expired.
			_ = conn.CloseWithError(xerrors.New("cache timeout"))
		}()
		c.connMutex.Lock()
		c.conns[id] = conn
		c.connMutex.Unlock()
	}
	conn.timeoutMutex.Lock()
	defer conn.timeoutMutex.Unlock()
	if conn.timeout != nil {
		conn.timeout.Stop()
	}
	conn.locks.Inc()
	return conn, func() {
		conn.locks.Dec()
		conn.timeoutMutex.Lock()
		defer conn.timeoutMutex.Unlock()
		if conn.timeout != nil {
			conn.timeout.Stop()
		}
		conn.timeout = time.AfterFunc(c.inactiveTimeout, conn.timeoutCancel)
	}, nil
}

func (c *Cache) Close() error {
	c.connMutex.Lock()
	defer c.connMutex.Unlock()
	for _, conn := range c.conns {
		_ = conn.Close()
	}
	return nil
}
