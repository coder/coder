package dispatch

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/notifications/render"
	"github.com/coder/coder/v2/coderd/notifications/types"
	markdown "github.com/coder/coder/v2/coderd/render"
	"github.com/coder/coder/v2/codersdk"
)

var (
	ValidationNoFromAddressErr   = xerrors.New("no 'from' address defined")
	ValidationNoToAddressErr     = xerrors.New("no 'to' address(es) defined")
	ValidationNoSmarthostHostErr = xerrors.New("smarthost 'host' is not defined, or is invalid")
	ValidationNoSmarthostPortErr = xerrors.New("smarthost 'port' is not defined, or is invalid")
	ValidationNoHelloErr         = xerrors.New("'hello' not defined")

	//go:embed smtp/html.gotmpl
	htmlTemplate string
	//go:embed smtp/plaintext.gotmpl
	plainTemplate string
)

// SMTPHandler is responsible for dispatching notification messages via SMTP.
// NOTE: auth and TLS is currently *not* enabled in this initial thin slice.
// TODO: implement auth
// TODO: implement TLS
type SMTPHandler struct {
	cfg codersdk.NotificationsEmailConfig
	log slog.Logger
}

func NewSMTPHandler(cfg codersdk.NotificationsEmailConfig, log slog.Logger) *SMTPHandler {
	return &SMTPHandler{cfg: cfg, log: log}
}

func (*SMTPHandler) NotificationMethod() database.NotificationMethod {
	return database.NotificationMethodSmtp
}

func (s *SMTPHandler) Dispatcher(payload types.MessagePayload, titleTmpl, bodyTmpl string) (DeliveryFunc, error) {
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
	htmlBody, err = render.GoTemplate(htmlTemplate, payload, nil)
	if err != nil {
		return nil, xerrors.Errorf("render full html template: %w", err)
	}
	payload.Labels["_body"] = plainBody
	plainBody, err = render.GoTemplate(plainTemplate, payload, nil)
	if err != nil {
		return nil, xerrors.Errorf("render full plaintext template: %w", err)
	}

	return s.dispatch(subject, htmlBody, plainBody, payload.UserEmail), nil
}

// dispatch returns a DeliveryFunc capable of delivering a notification via SMTP.
//
// NOTE: this is heavily inspired by Alertmanager's email notifier:
// https://github.com/prometheus/alertmanager/blob/342f6a599ce16c138663f18ed0b880e777c3017d/notify/email/email.go
func (s *SMTPHandler) dispatch(subject, htmlBody, plainBody, to string) DeliveryFunc {
	return func(ctx context.Context, msgID uuid.UUID) (bool, error) {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}

		var (
			c    *smtp.Client
			conn net.Conn
			err  error
		)

		s.log.Debug(ctx, "dispatching via SMTP", slog.F("msg_id", msgID))

		// Dial the smarthost to establish a connection.
		smarthost, smarthostPort, err := s.smarthost()
		if err != nil {
			return false, xerrors.Errorf("'smarthost' validation: %w", err)
		}
		if smarthostPort == "465" {
			return false, xerrors.New("TLS is not currently supported")
		}

		var d net.Dialer
		// Outer context has a deadline (see CODER_NOTIFICATIONS_DISPATCH_TIMEOUT).
		conn, err = d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%s", smarthost, smarthostPort))
		if err != nil {
			return true, xerrors.Errorf("establish connection to server: %w", err)
		}

		// Create an SMTP client.
		c, err = smtp.NewClient(conn, smarthost)
		if err != nil {
			if cerr := conn.Close(); cerr != nil {
				s.log.Warn(ctx, "failed to close connection", slog.Error(cerr))
			}
			return true, xerrors.Errorf("create client: %w", err)
		}

		// Cleanup.
		defer func() {
			if err := c.Quit(); err != nil {
				s.log.Warn(ctx, "failed to close SMTP connection", slog.Error(err))
			}
		}()

		// Server handshake.
		hello, err := s.hello()
		if err != nil {
			return false, xerrors.Errorf("'hello' validation: %w", err)
		}
		err = c.Hello(hello)
		if err != nil {
			return false, xerrors.Errorf("server handshake: %w", err)
		}

		// Check for authentication capabilities.
		// if ok, mech := c.Extension("AUTH"); ok {
		//	auth, err := s.auth(mech)
		//	if err != nil {
		//		return true, xerrors.Errorf("find auth mechanism: %w", err)
		//	}
		//	if auth != nil {
		//		if err := c.Auth(auth); err != nil {
		//			return true, xerrors.Errorf("%T auth: %w", auth, err)
		//		}
		//	}
		//}

		// Sender identification.
		from, err := s.validateFromAddr(s.cfg.From.String())
		if err != nil {
			return false, xerrors.Errorf("'from' validation: %w", err)
		}
		err = c.Mail(from)
		if err != nil {
			// This is retryable because the server may be temporarily down.
			return true, xerrors.Errorf("sender identification: %w", err)
		}

		// Recipient designation.
		to, err := s.validateToAddrs(to)
		if err != nil {
			return false, xerrors.Errorf("'to' validation: %w", err)
		}
		for _, addr := range to {
			err = c.Rcpt(addr)
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
		defer message.Close()

		// Transmit message headers.
		msg := &bytes.Buffer{}
		multipartBuffer := &bytes.Buffer{}
		multipartWriter := multipart.NewWriter(multipartBuffer)
		_, _ = fmt.Fprintf(msg, "From: %s\r\n", from)
		_, _ = fmt.Fprintf(msg, "To: %s\r\n", strings.Join(to, ", "))
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

		// Returning false, nil indicates successful send (i.e. non-retryable non-error)
		return false, nil
	}
}

// auth returns a value which implements the smtp.Auth based on the available auth mechanism.
// func (*SMTPHandler) auth(_ string) (smtp.Auth, error) {
//	return nil, nil
//}

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
	host := s.cfg.Smarthost.Host
	port := s.cfg.Smarthost.Port

	// We don't validate the contents themselves; this will be done by the underlying SMTP library.
	if host == "" {
		return "", "", ValidationNoSmarthostHostErr
	}
	if port == "" {
		return "", "", ValidationNoSmarthostPortErr
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
