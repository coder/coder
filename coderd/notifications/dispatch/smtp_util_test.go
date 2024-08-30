package dispatch_test

import (
	"crypto/tls"
	_ "embed"
	"io"
	"net"
	"sync"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"golang.org/x/xerrors"
)

// TLS cert files.
var (
	//go:embed fixtures/server.crt
	certFile []byte
	//go:embed fixtures/server.key
	keyFile []byte
)

type Config struct {
	AuthMechanisms                                       []string
	AcceptedIdentity, AcceptedUsername, AcceptedPassword string
	FailOnDataFn                                         func() error
}

type Message struct {
	AuthMech                     string
	Identity, Username, Password string // Auth
	From                         string
	To                           []string // Address
	Subject, Contents            string   // Content
}

type Backend struct {
	cfg Config

	mu      sync.Mutex
	lastMsg *Message
}

func NewBackend(cfg Config) *Backend {
	return &Backend{
		cfg: cfg,
	}
}

// NewSession is called after client greeting (EHLO, HELO).
func (b *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &Session{conn: c, backend: b}, nil
}

func (b *Backend) LastMessage() *Message {
	return b.lastMsg
}

func (b *Backend) Reset() {
	b.lastMsg = nil
}

type Session struct {
	conn    *smtp.Conn
	backend *Backend
}

// AuthMechanisms returns a slice of available auth mechanisms; only PLAIN is
// supported in this example.
func (s *Session) AuthMechanisms() []string {
	return s.backend.cfg.AuthMechanisms
}

// Auth is the handler for supported authenticators.
func (s *Session) Auth(mech string) (sasl.Server, error) {
	s.backend.mu.Lock()
	defer s.backend.mu.Unlock()

	if s.backend.lastMsg == nil {
		s.backend.lastMsg = &Message{AuthMech: mech}
	}

	switch mech {
	case sasl.Plain:
		return sasl.NewPlainServer(func(identity, username, password string) error {
			s.backend.lastMsg.Identity = identity
			s.backend.lastMsg.Username = username
			s.backend.lastMsg.Password = password

			if s.backend.cfg.AcceptedIdentity != "" && identity != s.backend.cfg.AcceptedIdentity {
				return xerrors.Errorf("unknown identity: %q", identity)
			}
			if username != s.backend.cfg.AcceptedUsername {
				return xerrors.Errorf("unknown user: %q", username)
			}
			if password != s.backend.cfg.AcceptedPassword {
				return xerrors.Errorf("incorrect password for username: %q", username)
			}

			return nil
		}), nil
	case sasl.Login:
		return sasl.NewLoginServer(func(username, password string) error {
			s.backend.lastMsg.Username = username
			s.backend.lastMsg.Password = password

			if username != s.backend.cfg.AcceptedUsername {
				return xerrors.Errorf("unknown user: %q", username)
			}
			if password != s.backend.cfg.AcceptedPassword {
				return xerrors.Errorf("incorrect password for username: %q", username)
			}

			return nil
		}), nil
	default:
		return nil, xerrors.Errorf("unexpected auth mechanism: %q", mech)
	}
}

func (s *Session) Mail(from string, _ *smtp.MailOptions) error {
	s.backend.mu.Lock()
	defer s.backend.mu.Unlock()

	if s.backend.lastMsg == nil {
		s.backend.lastMsg = &Message{}
	}

	s.backend.lastMsg.From = from
	return nil
}

func (s *Session) Rcpt(to string, _ *smtp.RcptOptions) error {
	s.backend.mu.Lock()
	defer s.backend.mu.Unlock()

	s.backend.lastMsg.To = append(s.backend.lastMsg.To, to)
	return nil
}

func (s *Session) Data(r io.Reader) error {
	s.backend.mu.Lock()
	defer s.backend.mu.Unlock()

	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	if s.backend.cfg.FailOnDataFn != nil {
		return s.backend.cfg.FailOnDataFn()
	}

	s.backend.lastMsg.Contents = string(b)

	return nil
}

func (*Session) Reset() {}

func (*Session) Logout() error { return nil }

// nolint:revive // Yes, useTLS is a control flag.
func createMockSMTPServer(be *Backend, useTLS bool) (*smtp.Server, net.Listener, error) {
	// nolint:gosec
	tlsCfg := &tls.Config{
		GetCertificate: readCert,
	}

	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, nil, xerrors.Errorf("connect: tls? %v: %w", useTLS, err)
	}

	if useTLS {
		l = tls.NewListener(l, tlsCfg)
	}

	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return nil, nil, xerrors.Errorf("unexpected address type: %T", l.Addr())
	}

	s := smtp.NewServer(be)

	s.Addr = addr.String()
	s.WriteTimeout = 10 * time.Second
	s.ReadTimeout = 10 * time.Second
	s.MaxMessageBytes = 1024 * 1024
	s.MaxRecipients = 50
	s.AllowInsecureAuth = !useTLS
	s.TLSConfig = tlsCfg

	return s, l, nil
}

func readCert(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	crt, err := tls.X509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, xerrors.Errorf("load x509 cert: %w", err)
	}

	return &crt, nil
}
