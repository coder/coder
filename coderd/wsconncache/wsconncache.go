// Package wsconncache caches workspace agent connections by UUID.
// Deprecated: Use ServerTailnet instead.
package wsconncache

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/atomic"
	"golang.org/x/sync/singleflight"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/site"
)

var _ workspaceapps.AgentProvider = (*AgentProvider)(nil)

type AgentProvider struct {
	Cache *Cache
}

func (a *AgentProvider) AgentConn(_ context.Context, agentID uuid.UUID) (*codersdk.WorkspaceAgentConn, func(), error) {
	conn, rel, err := a.Cache.Acquire(agentID)
	if err != nil {
		return nil, nil, xerrors.Errorf("acquire agent connection: %w", err)
	}

	return conn.WorkspaceAgentConn, rel, nil
}

func (a *AgentProvider) ReverseProxy(targetURL *url.URL, dashboardURL *url.URL, agentID uuid.UUID) (*httputil.ReverseProxy, func(), error) {
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		site.RenderStaticErrorPage(w, r, site.ErrorPageData{
			Status:       http.StatusBadGateway,
			Title:        "Bad Gateway",
			Description:  "Failed to proxy request to application: " + err.Error(),
			RetryEnabled: true,
			DashboardURL: dashboardURL.String(),
		})
	}

	conn, release, err := a.Cache.Acquire(agentID)
	if err != nil {
		return nil, nil, xerrors.Errorf("acquire agent connection: %w", err)
	}

	transport := conn.HTTPTransport()

	proxy.Transport = transport
	return proxy, release, nil
}

func (*AgentProvider) ServeHTTPDebug(http.ResponseWriter, *http.Request) {}

func (a *AgentProvider) Close() error {
	return a.Cache.Close()
}

// New creates a new workspace connection cache that closes connections after
// the inactive timeout provided.
//
// Agent connections are cached due to Wireguard negotiation taking a few
// hundred milliseconds, depending on latency.
//
// Deprecated: Use coderd.NewServerTailnet instead. wsconncache is being phased
// out because it creates a unique Tailnet for each agent.
// See: https://github.com/coder/coder/issues/8218
func New(dialer Dialer, inactiveTimeout time.Duration) *Cache {
	if inactiveTimeout == 0 {
		inactiveTimeout = 5 * time.Minute
	}
	return &Cache{
		closed:          make(chan struct{}),
		dialer:          dialer,
		inactiveTimeout: inactiveTimeout,
	}
}

// Dialer creates a new agent connection by ID.
type Dialer func(id uuid.UUID) (*codersdk.WorkspaceAgentConn, error)

// Conn wraps an agent connection with a reusable HTTP transport.
type Conn struct {
	*codersdk.WorkspaceAgentConn

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
	c.timeoutMutex.Lock()
	defer c.timeoutMutex.Unlock()
	if c.timeout != nil {
		c.timeout.Stop()
	}
	return c.WorkspaceAgentConn.Close()
}

type Cache struct {
	closed          chan struct{}
	closeMutex      sync.Mutex
	closeGroup      sync.WaitGroup
	connGroup       singleflight.Group
	connMap         sync.Map
	dialer          Dialer
	inactiveTimeout time.Duration
}

// Acquire gets or establishes a connection with the dialer using the ID provided.
// If a connection is in-progress, that connection or error will be returned.
//
// The returned function is used to release a lock on the connection. Once zero
// locks exist on a connection, the inactive timeout will begin to tick down.
// After the time expires, the connection will be cleared from the cache.
func (c *Cache) Acquire(id uuid.UUID) (*Conn, func(), error) {
	rawConn, found := c.connMap.Load(id.String())
	// If the connection isn't found, establish a new one!
	if !found {
		var err error
		// A singleflight group is used to allow for concurrent requests to the
		// same identifier to resolve.
		rawConn, err, _ = c.connGroup.Do(id.String(), func() (interface{}, error) {
			c.closeMutex.Lock()
			select {
			case <-c.closed:
				c.closeMutex.Unlock()
				return nil, xerrors.New("closed")
			default:
			}
			c.closeGroup.Add(1)
			c.closeMutex.Unlock()
			agentConn, err := c.dialer(id)
			if err != nil {
				c.closeGroup.Done()
				return nil, xerrors.Errorf("dial: %w", err)
			}
			timeoutCtx, timeoutCancelFunc := context.WithCancel(context.Background())
			defaultTransport, valid := http.DefaultTransport.(*http.Transport)
			if !valid {
				panic("dev error: default transport is the wrong type")
			}
			transport := defaultTransport.Clone()
			transport.DialContext = agentConn.DialContext

			// We intentionally don't verify the certificate chain here.
			// The connection to the workspace is already established and most
			// apps are already going to be accessed over plain HTTP, this config
			// simply allows apps being run over HTTPS to be accessed without error --
			// many of which may be using self-signed certs.
			transport.TLSClientConfig = &tls.Config{
				MinVersion: tls.VersionTLS12,
				//nolint:gosec
				InsecureSkipVerify: true,
			}

			conn := &Conn{
				WorkspaceAgentConn: agentConn,
				timeoutCancel:      timeoutCancelFunc,
				transport:          transport,
			}
			go func() {
				defer c.closeGroup.Done()
				select {
				case <-timeoutCtx.Done():
				case <-c.closed:
				case <-conn.Closed():
				}
				c.connMap.Delete(id.String())
				c.connGroup.Forget(id.String())
				transport.CloseIdleConnections()
				_ = conn.Close()
			}()
			return conn, nil
		})
		if err != nil {
			return nil, nil, err
		}
		c.connMap.Store(id.String(), rawConn)
	}

	conn, _ := rawConn.(*Conn)
	conn.timeoutMutex.Lock()
	defer conn.timeoutMutex.Unlock()
	if conn.timeout != nil {
		conn.timeout.Stop()
	}
	conn.locks.Inc()
	return conn, func() {
		conn.timeoutMutex.Lock()
		defer conn.timeoutMutex.Unlock()
		if conn.timeout != nil {
			conn.timeout.Stop()
		}
		conn.locks.Dec()
		if conn.locks.Load() == 0 {
			conn.timeout = time.AfterFunc(c.inactiveTimeout, conn.timeoutCancel)
		}
	}, nil
}

func (c *Cache) Close() error {
	c.closeMutex.Lock()
	defer c.closeMutex.Unlock()
	select {
	case <-c.closed:
		return nil
	default:
	}
	close(c.closed)
	c.closeGroup.Wait()
	return nil
}
