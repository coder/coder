package confine

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/agentsdk"
)

// unknownSNIHost marks denied TLS connections whose ClientHello did not yield
// a valid server name. An empty host cannot match an egress policy rule.
const unknownSNIHost = ""

var errClientHelloCaptured = xerrors.New("client hello captured")

// SNIListener forwards TLS connections after applying policy to ClientHello SNI.
type SNIListener struct {
	listener net.Listener
	policy   *PolicyEngine
	event    EventCallback
	closed   chan struct{}
	wg       sync.WaitGroup
}

// ListenSNI starts an SNI passthrough listener on address.
func ListenSNI(address string, policy *PolicyEngine, event EventCallback) (*SNIListener, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, xerrors.Errorf("listen sni proxy: %w", err)
	}
	server := &SNIListener{
		listener: listener,
		policy:   policy,
		event:    event,
		closed:   make(chan struct{}),
	}
	server.wg.Add(1)
	go server.serve()
	return server, nil
}

// Addr returns the SNI listener address.
func (s *SNIListener) Addr() net.Addr {
	return s.listener.Addr()
}

// Close stops the SNI listener and active accept loop.
func (s *SNIListener) Close() error {
	select {
	case <-s.closed:
	default:
		close(s.closed)
	}
	err := s.listener.Close()
	s.wg.Wait()
	return err
}

func (s *SNIListener) serve() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.closed:
				return
			default:
				continue
			}
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handle(conn)
		}()
	}
}

func (s *SNIListener) handle(client net.Conn) {
	defer client.Close()
	host, prefix, err := peekServerName(client)
	if err != nil {
		decision := s.policy.Decide(unknownSNIHost, defaultHTTPSPort)
		decision.Allowed = false
		s.emit(unknownSNIHost, decision)
		return
	}
	decision := s.policy.Decide(host, defaultHTTPSPort)
	s.emit(host, decision)
	if !decision.Allowed {
		return
	}

	upstream, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(defaultHTTPSPort)), 10*time.Second)
	if err != nil {
		return
	}
	splice(&prefixedConn{Conn: client, reader: io.MultiReader(bytes.NewReader(prefix), client)}, upstream)
}

func (s *SNIListener) emit(host string, decision Decision) {
	if s.event == nil {
		return
	}
	action := agentsdk.AISandboxNetworkEventActionDenied
	if decision.Allowed {
		action = agentsdk.AISandboxNetworkEventActionAllowed
	}
	s.event(NetworkEvent{
		Protocol:       agentsdk.AISandboxNetworkProtocolSNI,
		Host:           normalizeHost(host),
		Port:           defaultHTTPSPort,
		Action:         action,
		PolicyRevision: decision.Revision,
	})
}

type captureConn struct {
	net.Conn
	read bytes.Buffer
}

func (c *captureConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		_, _ = c.read.Write(p[:n])
	}
	return n, err
}

func (*captureConn) Write(p []byte) (int, error) {
	return len(p), nil
}

func peekServerName(conn net.Conn) (string, []byte, error) {
	capture := &captureConn{Conn: conn}
	var serverName string
	server := tls.Server(capture, &tls.Config{
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			serverName = hello.ServerName
			return nil, errClientHelloCaptured
		},
		MinVersion: tls.VersionTLS12,
	})
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	err := server.Handshake()
	_ = conn.SetReadDeadline(time.Time{})
	if !errors.Is(err, errClientHelloCaptured) {
		return "", nil, xerrors.Errorf("read tls client hello: %w", err)
	}
	if normalizeHost(serverName) == "" {
		return "", nil, xerrors.New("tls client hello has no server name")
	}
	return normalizeHost(serverName), append([]byte(nil), capture.read.Bytes()...), nil
}

type prefixedConn struct {
	net.Conn
	reader io.Reader
}

func (c *prefixedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}
