package dispatch

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"fmt"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/textproto"
	"os"
	"slices"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/emersion/go-sasl"
	smtp "github.com/emersion/go-smtp"
	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/notifications/render"
	"github.com/coder/coder/v2/coderd/notifications/types"
	markdown "github.com/coder/coder/v2/coderd/render"
	"github.com/coder/coder/v2/codersdk"
)

var (
	ValidationNoFromAddressErr = xerrors.New("'from' address not defined")
	ValidationNoToAddressErr   = xerrors.New("'to' address(es) not defined")
	ValidationNoSmarthostErr   = xerrors.New("'smarthost' address not defined")
	ValidationNoHelloErr       = xerrors.New("'hello' not defined")

	//go:embed smtp/html.gotmpl
	htmlTemplate string
	//go:embed smtp/plaintext.gotmpl
	plainTemplate string
)

// SMTPHandler is responsible for dispatching notification messages via SMTP.
// NOTE: auth and TLS is currently *not* enabled in this initial thin slice.
// TODO: implement DKIM/SPF/DMARC? https://github.com/emersion/go-msgauth
type SMTPHandler struct {
	cfg codersdk.NotificationsEmailConfig
	log slog.Logger

	noAuthWarnOnce sync.Once
	loginWarnOnce  sync.Once
}

func NewSMTPHandler(cfg codersdk.NotificationsEmailConfig, log slog.Logger) *SMTPHandler {
	return &SMTPHandler{cfg: cfg, log: log}
}

func (s *SMTPHandler) Dispatcher(payload types.MessagePayload, titleTmpl, bodyTmpl string, helpers template.FuncMap) (DeliveryFunc, error) {
	// First render the subject & body into their own discrete strings.
	subject, err := markdown.PlaintextFromMarkdown(titleTmpl)
	if err != nil {
		return nil, xerrors.Errorf("render subject: %w", err)
	}

	htmlBody := markdown.HTMLFromMarkdown(bodyTmpl)
	plainBody, err := markdown.PlaintextFromMarkdown(bodyTmpl)
	if err != nil {
		return nil, xerrors.Errorf("render plaintext body: %w", err)
	}

	// Then, reuse these strings in the HTML & plain body templates.
	payload.Labels["_subject"] = subject
	payload.Labels["_body"] = htmlBody
	htmlBody, err = render.GoTemplate(htmlTemplate, payload, helpers)
	if err != nil {
		return nil, xerrors.Errorf("render full html template: %w", err)
	}
	payload.Labels["_body"] = plainBody
	plainBody, err = render.GoTemplate(plainTemplate, payload, helpers)
	if err != nil {
		return nil, xerrors.Errorf("render full plaintext template: %w", err)
	}

	return s.dispatch(subject, htmlBody, plainBody, payload.UserEmail), nil
}

// dispatch returns a DeliveryFunc capable of delivering a notification via SMTP.
//
// Our requirements are too complex to be implemented using smtp.SendMail:
//   - we require custom TLS settings
//   - dynamic determination of available AUTH mechanisms
//
// NOTE: this is inspired by Alertmanager's email notifier:
// https://github.com/prometheus/alertmanager/blob/342f6a599ce16c138663f18ed0b880e777c3017d/notify/email/email.go
func (s *SMTPHandler) dispatch(subject, htmlBody, plainBody, to string) DeliveryFunc {
	return func(ctx context.Context, msgID uuid.UUID) (bool, error) {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}

		s.log.Debug(ctx, "dispatching via SMTP", slog.F("msg_id", msgID))

		// Dial the smarthost to establish a connection.
		smarthost, smarthostPort, err := s.smarthost()
		if err != nil {
			return false, xerrors.Errorf("'smarthost' validation: %w", err)
		}

		// Outer context has a deadline (see CODER_NOTIFICATIONS_DISPATCH_TIMEOUT).
		if _, ok := ctx.Deadline(); !ok {
			return false, xerrors.Errorf("context has no deadline")
		}

		// TODO: reuse client across dispatches (if possible).
		// Create an SMTP client for communication with the smarthost.
		c, err := s.client(ctx, smarthost, smarthostPort)
		if err != nil {
			return true, xerrors.Errorf("SMTP client creation: %w", err)
		}

		// Cleanup.
		defer func() {
			if err := c.Quit(); err != nil {
				s.log.Warn(ctx, "failed to close SMTP connection", slog.Error(err))
			}
		}()

		// Check for authentication capabilities.
		if ok, avail := c.Extension("AUTH"); ok {
			// Ensure the auth mechanisms available are ones we can use, and create a SASL client.
			auth, err := s.auth(ctx, avail)
			if err != nil {
				return true, xerrors.Errorf("determine auth mechanism: %w", err)
			}

			if auth == nil {
				// If we get here, no SASL client (which handles authentication) was returned.
				// This is expected if auth is supported by the smarthost BUT no authentication details were configured.
				s.noAuthWarnOnce.Do(func() {
					s.log.Warn(ctx, "skipping auth; no authentication client created")
				})
			} else {
				// We have a SASL client, use it to authenticate.
				if err := c.Auth(auth); err != nil {
					return true, xerrors.Errorf("%T auth: %w", auth, err)
				}
			}
		} else if !s.cfg.Auth.Empty() {
			return false, xerrors.New("no authentication mechanisms supported by server")
		}

		// Sender identification.
		from, err := s.validateFromAddr(s.cfg.From.String())
		if err != nil {
			return false, xerrors.Errorf("'from' validation: %w", err)
		}
		err = c.Mail(from, &smtp.MailOptions{})
		if err != nil {
			// This is retryable because the server may be temporarily down.
			return true, xerrors.Errorf("sender identification: %w", err)
		}

		// Recipient designation.
		recipients, err := s.validateToAddrs(to)
		if err != nil {
			return false, xerrors.Errorf("'to' validation: %w", err)
		}
		for _, addr := range recipients {
			err = c.Rcpt(addr, &smtp.RcptOptions{})
			if err != nil {
				// This is a retryable case because the server may be temporarily down.
				// The addresses are already validated, although it is possible that the server might disagree - in which case
				// this will lead to some spurious retries, but that's not a big deal.
				return true, xerrors.Errorf("recipient designation: %w", err)
			}
		}

		// Start message transmission.
		message, err := c.Data()
		if err != nil {
			return true, xerrors.Errorf("message transmission: %w", err)
		}
		closeOnce := sync.OnceValue(func() error {
			return message.Close()
		})
		// Close the message when this method exits in order to not leak resources. Even though we're calling this explicitly
		// further down, the method may exit before then.
		defer func() {
			// If we try close an already-closed writer, it'll send a subsequent request to the server which is invalid.
			_ = closeOnce()
		}()

		// Create message headers.
		msg := &bytes.Buffer{}
		multipartBuffer := &bytes.Buffer{}
		multipartWriter := multipart.NewWriter(multipartBuffer)
		_, _ = fmt.Fprintf(msg, "From: %s\r\n", from)
		_, _ = fmt.Fprintf(msg, "To: %s\r\n", strings.Join(recipients, ", "))
		_, _ = fmt.Fprintf(msg, "Subject: %s\r\n", subject)
		_, _ = fmt.Fprintf(msg, "Message-Id: %s@%s\r\n", msgID, s.hostname())
		_, _ = fmt.Fprintf(msg, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
		_, _ = fmt.Fprintf(msg, "Content-Type: multipart/alternative;  boundary=%s\r\n", multipartWriter.Boundary())
		_, _ = fmt.Fprintf(msg, "MIME-Version: 1.0\r\n\r\n")
		_, err = message.Write(msg.Bytes())
		if err != nil {
			return false, xerrors.Errorf("write headers: %w", err)
		}

		// Transmit message body.

		// Text body
		w, err := multipartWriter.CreatePart(textproto.MIMEHeader{
			"Content-Transfer-Encoding": {"quoted-printable"},
			"Content-Type":              {"text/plain; charset=UTF-8"},
		})
		if err != nil {
			return false, xerrors.Errorf("create part for text body: %w", err)
		}
		qw := quotedprintable.NewWriter(w)
		_, err = qw.Write([]byte(plainBody))
		if err != nil {
			return true, xerrors.Errorf("write text part: %w", err)
		}
		err = qw.Close()
		if err != nil {
			return true, xerrors.Errorf("close text part: %w", err)
		}

		// HTML body
		// Preferred body placed last per section 5.1.4 of RFC 2046
		// https://www.ietf.org/rfc/rfc2046.txt
		w, err = multipartWriter.CreatePart(textproto.MIMEHeader{
			"Content-Transfer-Encoding": {"quoted-printable"},
			"Content-Type":              {"text/html; charset=UTF-8"},
		})
		if err != nil {
			return false, xerrors.Errorf("create part for HTML body: %w", err)
		}
		qw = quotedprintable.NewWriter(w)
		_, err = qw.Write([]byte(htmlBody))
		if err != nil {
			return true, xerrors.Errorf("write HTML part: %w", err)
		}
		err = qw.Close()
		if err != nil {
			return true, xerrors.Errorf("close HTML part: %w", err)
		}

		err = multipartWriter.Close()
		if err != nil {
			return false, xerrors.Errorf("close multipartWriter: %w", err)
		}

		_, err = message.Write(multipartBuffer.Bytes())
		if err != nil {
			return false, xerrors.Errorf("write body buffer: %w", err)
		}

		if err = closeOnce(); err != nil {
			return true, xerrors.Errorf("delivery failure: %w", err)
		}

		// Returning false, nil indicates successful send (i.e. non-retryable non-error)
		return false, nil
	}
}

// client creates an SMTP client capable of communicating over a plain or TLS-encrypted connection.
func (s *SMTPHandler) client(ctx context.Context, host string, port string) (*smtp.Client, error) {
	var (
		c    *smtp.Client
		conn net.Conn
		d    net.Dialer
		err  error
	)

	// Outer context has a deadline (see CODER_NOTIFICATIONS_DISPATCH_TIMEOUT).
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil, xerrors.Errorf("context has no deadline")
	}
	// Align with context deadline.
	d.Deadline = deadline

	tlsCfg, err := s.tlsConfig()
	if err != nil {
		return nil, xerrors.Errorf("build TLS config: %w", err)
	}

	smarthost := fmt.Sprintf("%s:%s", host, port)
	useTLS := false

	// Use TLS if known TLS port(s) are used or TLS is forced.
	if port == "465" || s.cfg.ForceTLS {
		useTLS = true

		// STARTTLS is only used on plain connections to upgrade.
		if s.cfg.TLS.StartTLS {
			s.log.Warn(ctx, "STARTTLS is not allowed on TLS connections; disabling STARTTLS")
			s.cfg.TLS.StartTLS = false
		}
	}

	// Dial a TLS or plain connection to the smarthost.
	if useTLS {
		conn, err = tls.DialWithDialer(&d, "tcp", smarthost, tlsCfg)
		if err != nil {
			return nil, xerrors.Errorf("establish TLS connection to server: %w", err)
		}
	} else {
		conn, err = d.DialContext(ctx, "tcp", smarthost)
		if err != nil {
			return nil, xerrors.Errorf("establish plain connection to server: %w", err)
		}
	}

	// If the connection is plain, and STARTTLS is configured, try to upgrade the connection.
	if s.cfg.TLS.StartTLS {
		c, err = smtp.NewClientStartTLS(conn, tlsCfg)
		if err != nil {
			return nil, xerrors.Errorf("upgrade connection with STARTTLS: %w", err)
		}
	} else {
		c = smtp.NewClient(conn)

		// HELO is performed here and not always because smtp.NewClientStartTLS greets the server already to establish
		// whether STARTTLS is allowed.

		var hello string
		// Server handshake.
		hello, err = s.hello()
		if err != nil {
			return nil, xerrors.Errorf("'hello' validation: %w", err)
		}
		err = c.Hello(hello)
		if err != nil {
			return nil, xerrors.Errorf("server handshake: %w", err)
		}
	}

	// Align with context deadline.
	c.CommandTimeout = time.Until(deadline)
	c.SubmissionTimeout = time.Until(deadline)

	return c, nil
}

func (s *SMTPHandler) tlsConfig() (*tls.Config, error) {
	host, _, err := s.smarthost()
	if err != nil {
		return nil, err
	}

	srvName := s.cfg.TLS.ServerName.String()
	if srvName == "" {
		srvName = host
	}

	ca, err := s.loadCAFile()
	if err != nil {
		return nil, xerrors.Errorf("load CA: %w", err)
	}

	var certs []tls.Certificate
	cert, err := s.loadCertificate()
	if err != nil {
		return nil, xerrors.Errorf("load cert: %w", err)
	}

	if cert != nil {
		certs = append(certs, *cert)
	}

	return &tls.Config{
		ServerName: srvName,
		// nolint:gosec // Users may choose to enable this.
		InsecureSkipVerify: s.cfg.TLS.InsecureSkipVerify.Value(),

		RootCAs:      ca,
		Certificates: certs,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}, nil
}

func (s *SMTPHandler) loadCAFile() (*x509.CertPool, error) {
	if s.cfg.TLS.CAFile == "" {
		// nolint:nilnil // A nil CertPool is a valid response.
		return nil, nil
	}

	ca, err := os.ReadFile(s.cfg.TLS.CAFile.String())
	if err != nil {
		return nil, xerrors.Errorf("load CA file: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, xerrors.Errorf("build cert pool: %w", err)
	}

	return pool, nil
}

func (s *SMTPHandler) loadCertificate() (*tls.Certificate, error) {
	if len(s.cfg.TLS.CertFile) == 0 && len(s.cfg.TLS.KeyFile) == 0 {
		// nolint:nilnil // A nil certificate is a valid response.
		return nil, nil
	}

	cert, err := os.ReadFile(s.cfg.TLS.CertFile.Value())
	if err != nil {
		return nil, xerrors.Errorf("load cert: %w", err)
	}
	key, err := os.ReadFile(s.cfg.TLS.KeyFile.String())
	if err != nil {
		return nil, xerrors.Errorf("load key: %w", err)
	}

	pair, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return nil, xerrors.Errorf("invalid or unusable keypair: %w", err)
	}

	return &pair, nil
}

// auth returns a value which implements the smtp.Auth based on the available auth mechanisms.
func (s *SMTPHandler) auth(ctx context.Context, mechs string) (sasl.Client, error) {
	username := s.cfg.Auth.Username.String()

	// All auth mechanisms require username, so if one is not defined then don't return an auth client.
	if username == "" {
		// nolint:nilnil // This is a valid response.
		return nil, nil
	}

	var errs error
	list := strings.Split(mechs, " ")
	for _, mech := range list {
		switch mech {
		case sasl.Plain:
			password, err := s.password()
			if err != nil {
				errs = multierror.Append(errs, err)
				continue
			}
			if password == "" {
				errs = multierror.Append(errs, xerrors.New("cannot use PLAIN auth, password not defined (see CODER_EMAIL_AUTH_PASSWORD)"))
				continue
			}

			return sasl.NewPlainClient(s.cfg.Auth.Identity.String(), username, password), nil
		case sasl.Login:
			if slices.Contains(list, sasl.Plain) {
				// Prefer PLAIN over LOGIN.
				continue
			}

			// Warn that LOGIN is obsolete, but don't do it every time we dispatch a notification.
			s.loginWarnOnce.Do(func() {
				s.log.Warn(ctx, "LOGIN auth is obsolete and should be avoided (use PLAIN instead): https://www.ietf.org/archive/id/draft-murchison-sasl-login-00.txt")
			})

			password, err := s.password()
			if err != nil {
				errs = multierror.Append(errs, err)
				continue
			}
			if password == "" {
				errs = multierror.Append(errs, xerrors.New("cannot use LOGIN auth, password not defined (see CODER_EMAIL_AUTH_PASSWORD)"))
				continue
			}

			return sasl.NewLoginClient(username, password), nil
		default:
			return nil, xerrors.Errorf("unsupported auth mechanism: %q (supported: %v)", mechs, []string{sasl.Plain, sasl.Login})
		}
	}

	return nil, errs
}

func (*SMTPHandler) validateFromAddr(from string) (string, error) {
	addrs, err := mail.ParseAddressList(from)
	if err != nil {
		return "", xerrors.Errorf("parse 'from' address: %w", err)
	}
	if len(addrs) != 1 {
		return "", ValidationNoFromAddressErr
	}
	return from, nil
}

func (s *SMTPHandler) validateToAddrs(to string) ([]string, error) {
	addrs, err := mail.ParseAddressList(to)
	if err != nil {
		return nil, xerrors.Errorf("parse 'to' addresses: %w", err)
	}
	if len(addrs) == 0 {
		s.log.Warn(context.Background(), "no valid 'to' address(es) defined; some may be invalid", slog.F("defined", to))
		return nil, ValidationNoToAddressErr
	}

	var out []string
	for _, addr := range addrs {
		out = append(out, addr.Address)
	}

	return out, nil
}

// smarthost retrieves the host/port defined and validates them.
// Does not allow overriding.
// nolint:revive // documented.
func (s *SMTPHandler) smarthost() (string, string, error) {
	smarthost := strings.TrimSpace(string(s.cfg.Smarthost))
	if smarthost == "" {
		return "", "", ValidationNoSmarthostErr
	}

	host, port, err := net.SplitHostPort(string(s.cfg.Smarthost))
	if err != nil {
		return "", "", xerrors.Errorf("split host port: %w", err)
	}

	return host, port, nil
}

// hello retrieves the hostname identifying the SMTP server.
// Does not allow overriding.
func (s *SMTPHandler) hello() (string, error) {
	val := s.cfg.Hello.String()
	if val == "" {
		return "", ValidationNoHelloErr
	}
	return val, nil
}

func (*SMTPHandler) hostname() string {
	h, err := os.Hostname()
	// If we can't get the hostname, we'll use localhost
	if err != nil {
		h = "localhost.localdomain"
	}
	return h
}

// password returns either the configured password, or reads it from the configured file (if possible).
func (s *SMTPHandler) password() (string, error) {
	file := s.cfg.Auth.PasswordFile.String()
	if len(file) > 0 {
		content, err := os.ReadFile(file)
		if err != nil {
			return "", xerrors.Errorf("could not read %s: %w", file, err)
		}
		return string(content), nil
	}
	return s.cfg.Auth.Password.String(), nil
}
