package smtpmock

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	smtpmocklib "github.com/mocktools/go-smtp-mock/v2"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

// Server wraps the SMTP mock server and provides an HTTP API to retrieve emails.
type Server struct {
	smtpServer *smtpmocklib.Server
	httpServer *http.Server
	listener   net.Listener
	logger     slog.Logger

	host     string
	smtpPort int
	apiPort  int
}

type Config struct {
	Host     string
	SMTPPort int
	APIPort  int
	Logger   slog.Logger
}

type EmailSummary struct {
	Subject        string    `json:"subject"`
	Date           time.Time `json:"date"`
	NotificationID uuid.UUID `json:"notification_id,omitempty"`
}

var notificationIDRegex = regexp.MustCompile(`notifications\?disabled=3D([a-f0-9-]+)`)

func New(cfg Config) *Server {
	return &Server{
		host:     cfg.Host,
		smtpPort: cfg.SMTPPort,
		apiPort:  cfg.APIPort,
		logger:   cfg.Logger,
	}
}

func (s *Server) Start(ctx context.Context) error {
	s.smtpServer = smtpmocklib.New(smtpmocklib.ConfigurationAttr{
		LogToStdout:       false,
		LogServerActivity: true,
		HostAddress:       s.host,
		PortNumber:        s.smtpPort,
	})
	if err := s.smtpServer.Start(); err != nil {
		return xerrors.Errorf("start SMTP server: %w", err)
	}
	s.smtpPort = s.smtpServer.PortNumber()

	if err := s.startAPIServer(ctx); err != nil {
		_ = s.smtpServer.Stop()
		return xerrors.Errorf("start API server: %w", err)
	}

	return nil
}

func (s *Server) Stop() error {
	var httpErr, listenerErr, smtpErr error

	if s.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			httpErr = xerrors.Errorf("shutdown HTTP server: %w", err)
		}
	}

	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			listenerErr = xerrors.Errorf("close listener: %w", err)
		}
	}

	if s.smtpServer != nil {
		if err := s.smtpServer.Stop(); err != nil {
			smtpErr = xerrors.Errorf("stop SMTP server: %w", err)
		}
	}

	return errors.Join(httpErr, listenerErr, smtpErr)
}

func (s *Server) SMTPAddress() string {
	return fmt.Sprintf("%s:%d", s.host, s.smtpPort)
}

func (s *Server) APIAddress() string {
	return fmt.Sprintf("%s:%d", s.host, s.apiPort)
}

func (s *Server) MessageCount() int {
	if s.smtpServer == nil {
		return 0
	}
	return len(s.smtpServer.Messages())
}

func (s *Server) Purge() {
	if s.smtpServer != nil {
		s.smtpServer.MessagesAndPurge()
	}
}

func (s *Server) startAPIServer(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /purge", s.handlePurge)
	mux.HandleFunc("GET /messages", s.handleMessages)

	s.httpServer = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.host, s.apiPort))
	if err != nil {
		return xerrors.Errorf("listen on %s:%d: %w", s.host, s.apiPort, err)
	}
	s.listener = listener

	tcpAddr, valid := listener.Addr().(*net.TCPAddr)
	if !valid {
		listener.Close()
		return xerrors.Errorf("listener returned invalid address: %T", listener.Addr())
	}
	s.apiPort = tcpAddr.Port

	go func() {
		if err := s.httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error(ctx, "http API server error", slog.Error(err))
		}
	}()

	return nil
}

func (s *Server) handlePurge(w http.ResponseWriter, _ *http.Request) {
	s.smtpServer.MessagesAndPurge()
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	msgs := s.smtpServer.Messages()

	var summaries []EmailSummary
	for _, msg := range msgs {
		recipients := msg.RcpttoRequestResponse()
		if !matchesRecipient(recipients, email) {
			continue
		}

		summary, err := parseEmailSummary(msg.MsgRequest())
		if err != nil {
			s.logger.Warn(r.Context(), "failed to parse email summary", slog.Error(err))
			continue
		}
		summaries = append(summaries, summary)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(summaries); err != nil {
		s.logger.Warn(r.Context(), "failed to encode JSON response", slog.Error(err))
	}
}

func matchesRecipient(recipients [][]string, email string) bool {
	if email == "" {
		return true
	}
	return slices.ContainsFunc(recipients, func(rcptPair []string) bool {
		return len(rcptPair) > 0 && strings.Contains(rcptPair[0], email)
	})
}

func parseEmailSummary(content string) (EmailSummary, error) {
	var summary EmailSummary
	scanner := bufio.NewScanner(strings.NewReader(content))

	// Extract Subject and Date from headers.
	// Date is used to measure latency.
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		if prefix, found := strings.CutPrefix(line, "Subject: "); found {
			summary.Subject = prefix
		} else if prefix, found := strings.CutPrefix(line, "Date: "); found {
			if parsedDate, err := time.Parse(time.RFC1123Z, prefix); err == nil {
				summary.Date = parsedDate
			}
		}
	}

	// Extract notification ID from email content
	// Notification ID is present in the email footer like this
	// <p><a href=3D"http://127.0.0.1:3000/settings/notifications?disabled=3D
	// =3D4e19c0ac-94e1-4532-9515-d1801aa283b2" style=3D"color: #2563eb; text-deco=
	// ration: none;">Stop receiving emails like this</a></p>
	replacer := strings.NewReplacer("=\n", "", "=\r\n", "")
	contentNormalized := replacer.Replace(content)
	if matches := notificationIDRegex.FindStringSubmatch(contentNormalized); len(matches) > 1 {
		var err error
		summary.NotificationID, err = uuid.Parse(matches[1])
		if err != nil {
			return summary, xerrors.Errorf("failed to parse notification ID: %w", err)
		}
	}

	return summary, nil
}
