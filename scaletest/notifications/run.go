package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/createusers"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/coder/v2/scaletest/smtpmock"
	"github.com/coder/websocket"
)

type Runner struct {
	client *codersdk.Client
	cfg    Config

	createUserRunner *createusers.Runner

	// websocketLatencies stores the latency for websocket notifications
	websocketLatencies   map[uuid.UUID]time.Duration
	websocketLatenciesMu sync.RWMutex

	// smtpLatencies stores the latency for SMTP notifications
	smtpLatencies   map[uuid.UUID]time.Duration
	smtpLatenciesMu sync.RWMutex
}

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	return &Runner{
		client:             client,
		cfg:                cfg,
		websocketLatencies: make(map[uuid.UUID]time.Duration),
		smtpLatencies:      make(map[uuid.UUID]time.Duration),
	}
}

var (
	_ harness.Runnable    = &Runner{}
	_ harness.Cleanable   = &Runner{}
	_ harness.Collectable = &Runner{}
)

func (r *Runner) Run(ctx context.Context, id string, logs io.Writer) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	reachedBarrier := false
	defer func() {
		if !reachedBarrier {
			r.cfg.DialBarrier.Done()
		}
	}()

	reachedReceivingWatchBarrier := false
	defer func() {
		if len(r.cfg.ExpectedNotifications) > 0 && !reachedReceivingWatchBarrier {
			r.cfg.ReceivingWatchBarrier.Done()
		}
	}()

	logs = loadtestutil.NewSyncWriter(logs)
	logger := slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug)
	r.client.SetLogger(logger)
	r.client.SetLogBodies(true)

	r.createUserRunner = createusers.NewRunner(r.client, r.cfg.User)
	newUserAndToken, err := r.createUserRunner.RunReturningUser(ctx, id, logs)
	if err != nil {
		r.cfg.Metrics.AddError("", "create_user")
		return xerrors.Errorf("create user: %w", err)
	}
	newUser := newUserAndToken.User
	newUserClient := codersdk.New(r.client.URL,
		codersdk.WithSessionToken(newUserAndToken.SessionToken),
		codersdk.WithLogger(logger),
		codersdk.WithLogBodies())

	logger.Info(ctx, "runner user created", slog.F("username", newUser.Username), slog.F("user_id", newUser.ID.String()))

	if len(r.cfg.Roles) > 0 {
		logger.Info(ctx, "assigning roles to user", slog.F("roles", r.cfg.Roles))

		_, err := r.client.UpdateUserRoles(ctx, newUser.ID.String(), codersdk.UpdateRoles{
			Roles: r.cfg.Roles,
		})
		if err != nil {
			r.cfg.Metrics.AddError(newUser.Username, "assign_roles")
			return xerrors.Errorf("assign roles: %w", err)
		}
	}

	logger.Info(ctx, "notification runner is ready")

	dialCtx, cancel := context.WithTimeout(ctx, r.cfg.DialTimeout)
	defer cancel()

	logger.Info(ctx, "connecting to notification websocket")
	conn, err := r.dialNotificationWebsocket(dialCtx, newUserClient, newUser, logger)
	if err != nil {
		return xerrors.Errorf("dial notification websocket: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")
	logger.Info(ctx, "connected to notification websocket")

	reachedBarrier = true
	r.cfg.DialBarrier.Done()
	r.cfg.DialBarrier.Wait()

	if len(r.cfg.ExpectedNotifications) == 0 {
		logger.Info(ctx, "maintaining websocket connection, waiting for receiving users to complete")

		// Wait for receiving users to complete
		done := make(chan struct{})
		go func() {
			r.cfg.ReceivingWatchBarrier.Wait()
			close(done)
		}()

		select {
		case <-done:
			logger.Info(ctx, "receiving users complete, closing connection")
		case <-ctx.Done():
			logger.Info(ctx, "context canceled, closing connection")
		}
		return nil
	}

	logger.Info(ctx, "waiting for notifications", slog.F("timeout", r.cfg.NotificationTimeout))

	watchCtx, cancel := context.WithTimeout(ctx, r.cfg.NotificationTimeout)
	defer cancel()

	eg, egCtx := errgroup.WithContext(watchCtx)

	eg.Go(func() error {
		return r.watchNotifications(egCtx, conn, newUser, logger, r.cfg.ExpectedNotifications)
	})

	if r.cfg.SMTPApiURL != "" {
		logger.Info(ctx, "running SMTP notification watcher")
		eg.Go(func() error {
			return r.watchNotificationsSMTP(egCtx, newUser, logger, r.cfg.ExpectedNotifications)
		})
	}

	if err := eg.Wait(); err != nil {
		return xerrors.Errorf("notification watch failed: %w", err)
	}

	reachedReceivingWatchBarrier = true
	r.cfg.ReceivingWatchBarrier.Done()

	return nil
}

func (r *Runner) Cleanup(ctx context.Context, id string, logs io.Writer) error {
	if r.createUserRunner != nil {
		_, _ = fmt.Fprintln(logs, "Cleaning up user...")
		if err := r.createUserRunner.Cleanup(ctx, id, logs); err != nil {
			return xerrors.Errorf("cleanup user: %w", err)
		}
	}

	return nil
}

const (
	WebsocketNotificationLatencyMetric = "notification_websocket_latency_seconds"
	SMTPNotificationLatencyMetric      = "notification_smtp_latency_seconds"
)

func (r *Runner) GetMetrics() map[string]any {
	r.websocketLatenciesMu.RLock()
	websocketLatencies := make(map[uuid.UUID]time.Duration, len(r.websocketLatencies))
	for id, latency := range r.websocketLatencies {
		websocketLatencies[id] = latency
	}
	r.websocketLatenciesMu.RUnlock()

	r.smtpLatenciesMu.RLock()
	smtpLatencies := make(map[uuid.UUID]time.Duration, len(r.smtpLatencies))
	for id, latency := range r.smtpLatencies {
		smtpLatencies[id] = latency
	}
	r.smtpLatenciesMu.RUnlock()

	return map[string]any{
		WebsocketNotificationLatencyMetric: websocketLatencies,
		SMTPNotificationLatencyMetric:      smtpLatencies,
	}
}

func (r *Runner) dialNotificationWebsocket(ctx context.Context, client *codersdk.Client, user codersdk.User, logger slog.Logger) (*websocket.Conn, error) {
	u, err := client.URL.Parse("/api/v2/notifications/inbox/watch")
	if err != nil {
		logger.Error(ctx, "parse notification URL", slog.Error(err))
		r.cfg.Metrics.AddError(user.Username, "parse_url")
		return nil, xerrors.Errorf("parse notification URL: %w", err)
	}

	conn, resp, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Coder-Session-Token": []string{client.SessionToken()},
		},
	})
	if err != nil {
		if resp != nil {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusSwitchingProtocols {
				err = codersdk.ReadBodyAsError(resp)
			}
		}
		logger.Error(ctx, "dial notification websocket", slog.Error(err))
		r.cfg.Metrics.AddError(user.Username, "dial")
		return nil, xerrors.Errorf("dial notification websocket: %w", err)
	}

	return conn, nil
}

// watchNotifications reads notifications from the websocket and returns error or nil
// once all expected notifications are received.
func (r *Runner) watchNotifications(ctx context.Context, conn *websocket.Conn, user codersdk.User, logger slog.Logger, expectedNotifications map[uuid.UUID]chan time.Time) error {
	logger.Info(ctx, "waiting for notifications",
		slog.F("username", user.Username),
		slog.F("expected_count", len(expectedNotifications)))

	receivedNotifications := make(map[uuid.UUID]struct{})

	for {
		select {
		case <-ctx.Done():
			return xerrors.Errorf("context canceled while waiting for notifications: %w", ctx.Err())
		default:
		}

		if len(receivedNotifications) == len(expectedNotifications) {
			logger.Info(ctx, "received all expected notifications")
			return nil
		}

		notif, err := readNotification(ctx, conn)
		if err != nil {
			logger.Error(ctx, "read notification", slog.Error(err))
			r.cfg.Metrics.AddError(user.Username, "read_notification")
			return xerrors.Errorf("read notification: %w", err)
		}

		templateID := notif.Notification.TemplateID
		if triggerTimeChan, exists := expectedNotifications[templateID]; exists {
			if _, exists := receivedNotifications[templateID]; !exists {
				receiptTime := time.Now()
				select {
				case triggerTime := <-triggerTimeChan:
					latency := receiptTime.Sub(triggerTime)
					r.websocketLatenciesMu.Lock()
					r.websocketLatencies[templateID] = latency
					r.websocketLatenciesMu.Unlock()
					r.cfg.Metrics.RecordLatency(latency, user.Username, templateID.String(), NotificationTypeWebsocket)
					receivedNotifications[templateID] = struct{}{}

					logger.Info(ctx, "received expected notification",
						slog.F("template_id", templateID),
						slog.F("title", notif.Notification.Title),
						slog.F("latency", latency))
				case <-ctx.Done():
					return xerrors.Errorf("context canceled while waiting for trigger time: %w", ctx.Err())
				}
			}
		} else {
			logger.Debug(ctx, "received notification not being tested",
				slog.F("template_id", templateID),
				slog.F("title", notif.Notification.Title))
		}
	}
}

const smtpPollInterval = 2 * time.Second

// watchNotificationsSMTP polls the SMTP HTTP API for notifications and returns error or nil
// once all expected notifications are received.
func (r *Runner) watchNotificationsSMTP(ctx context.Context, user codersdk.User, logger slog.Logger, expectedNotifications map[uuid.UUID]chan time.Time) error {
	logger.Info(ctx, "polling SMTP API for notifications",
		slog.F("username", user.Username),
		slog.F("email", user.Email),
		slog.F("expected_count", len(expectedNotifications)),
	)

	receivedNotifications := make(map[uuid.UUID]struct{})
	ticker := time.NewTicker(smtpPollInterval)
	defer ticker.Stop()

	apiURL := fmt.Sprintf("%s/messages?email=%s", r.cfg.SMTPApiURL, user.Email)
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	for {
		select {
		case <-ctx.Done():
			return xerrors.Errorf("context canceled while waiting for notifications: %w", ctx.Err())
		case <-ticker.C:
			if len(receivedNotifications) == len(expectedNotifications) {
				logger.Info(ctx, "received all expected notifications via SMTP")
				return nil
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
			if err != nil {
				logger.Error(ctx, "create SMTP API request", slog.Error(err))
				r.cfg.Metrics.AddError(user.Username, "smtp_create_request")
				return xerrors.Errorf("create SMTP API request: %w", err)
			}

			resp, err := httpClient.Do(req)
			if err != nil {
				logger.Error(ctx, "poll smtp api for notifications", slog.Error(err))
				r.cfg.Metrics.AddError(user.Username, "smtp_poll")
				return xerrors.Errorf("poll smtp api: %w", err)
			}

			if resp.StatusCode != http.StatusOK {
				_ = resp.Body.Close()
				logger.Error(ctx, "smtp api returned non-200 status", slog.F("status", resp.StatusCode))
				r.cfg.Metrics.AddError(user.Username, "smtp_bad_status")
				return xerrors.Errorf("smtp api returned status %d", resp.StatusCode)
			}

			var summaries []smtpmock.EmailSummary
			if err := json.NewDecoder(resp.Body).Decode(&summaries); err != nil {
				_ = resp.Body.Close()
				logger.Error(ctx, "decode smtp api response", slog.Error(err))
				r.cfg.Metrics.AddError(user.Username, "smtp_decode")
				return xerrors.Errorf("decode smtp api response: %w", err)
			}
			_ = resp.Body.Close()

			// Process each email summary
			for _, summary := range summaries {
				notificationID := summary.NotificationTemplateID
				if notificationID == uuid.Nil {
					continue
				}

				if triggerTimeChan, exists := expectedNotifications[notificationID]; exists {
					if _, received := receivedNotifications[notificationID]; !received {
						receiptTime := summary.Date
						if receiptTime.IsZero() {
							receiptTime = time.Now()
						}

						select {
						case triggerTime := <-triggerTimeChan:
							latency := receiptTime.Sub(triggerTime)
							r.smtpLatenciesMu.Lock()
							r.smtpLatencies[notificationID] = latency
							r.smtpLatenciesMu.Unlock()
							r.cfg.Metrics.RecordLatency(latency, user.Username, notificationID.String(), NotificationTypeSMTP)
							receivedNotifications[notificationID] = struct{}{}

							logger.Info(ctx, "received expected notification via SMTP",
								slog.F("notification_id", notificationID),
								slog.F("subject", summary.Subject),
								slog.F("latency", latency))
						case <-ctx.Done():
							return xerrors.Errorf("context canceled while waiting for trigger time: %w", ctx.Err())
						default:
						}
					}
				}
			}
		}
	}
}

func readNotification(ctx context.Context, conn *websocket.Conn) (codersdk.GetInboxNotificationResponse, error) {
	_, message, err := conn.Read(ctx)
	if err != nil {
		return codersdk.GetInboxNotificationResponse{}, err
	}

	var notif codersdk.GetInboxNotificationResponse
	if err := json.Unmarshal(message, &notif); err != nil {
		return codersdk.GetInboxNotificationResponse{}, xerrors.Errorf("unmarshal notification: %w", err)
	}

	return notif, nil
}
