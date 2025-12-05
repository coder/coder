package alerts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
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
	"github.com/coder/quartz"
	"github.com/coder/websocket"
)

type Runner struct {
	client *codersdk.Client
	cfg    Config

	createUserRunner *createusers.Runner

	// websocketReceiptTimes stores the receipt time for websocket notifications
	websocketReceiptTimes   map[uuid.UUID]time.Time
	websocketReceiptTimesMu sync.RWMutex

	// smtpReceiptTimes stores the receipt time for SMTP notifications
	smtpReceiptTimes   map[uuid.UUID]time.Time
	smtpReceiptTimesMu sync.RWMutex

	clock quartz.Clock
}

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	return &Runner{
		client:                client,
		cfg:                   cfg,
		websocketReceiptTimes: make(map[uuid.UUID]time.Time),
		smtpReceiptTimes:      make(map[uuid.UUID]time.Time),
		clock:                 quartz.NewReal(),
	}
}

func (r *Runner) WithClock(clock quartz.Clock) *Runner {
	r.clock = clock
	return r
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
		if len(r.cfg.ExpectedNotificationsIDs) > 0 && !reachedReceivingWatchBarrier {
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
		r.cfg.Metrics.AddError("create_user")
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
			r.cfg.Metrics.AddError("assign_roles")
			return xerrors.Errorf("assign roles: %w", err)
		}
	}

	logger.Info(ctx, "notification runner is ready")

	dialCtx, cancel := context.WithTimeout(ctx, r.cfg.DialTimeout)
	defer cancel()

	logger.Info(ctx, "connecting to notification websocket")
	conn, err := r.dialNotificationWebsocket(dialCtx, newUserClient, logger)
	if err != nil {
		return xerrors.Errorf("dial notification websocket: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")
	logger.Info(ctx, "connected to notification websocket")

	reachedBarrier = true
	r.cfg.DialBarrier.Done()
	r.cfg.DialBarrier.Wait()

	if len(r.cfg.ExpectedNotificationsIDs) == 0 {
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
		return r.watchNotifications(egCtx, conn, newUser, logger, r.cfg.ExpectedNotificationsIDs)
	})

	if r.cfg.SMTPApiURL != "" {
		logger.Info(ctx, "running SMTP notification watcher")
		eg.Go(func() error {
			return r.watchNotificationsSMTP(egCtx, newUser, logger, r.cfg.ExpectedNotificationsIDs)
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
	WebsocketNotificationReceiptTimeMetric = "notification_websocket_receipt_time"
	SMTPNotificationReceiptTimeMetric      = "notification_smtp_receipt_time"
)

func (r *Runner) GetMetrics() map[string]any {
	r.websocketReceiptTimesMu.RLock()
	websocketReceiptTimes := maps.Clone(r.websocketReceiptTimes)
	r.websocketReceiptTimesMu.RUnlock()

	r.smtpReceiptTimesMu.RLock()
	smtpReceiptTimes := maps.Clone(r.smtpReceiptTimes)
	r.smtpReceiptTimesMu.RUnlock()

	return map[string]any{
		WebsocketNotificationReceiptTimeMetric: websocketReceiptTimes,
		SMTPNotificationReceiptTimeMetric:      smtpReceiptTimes,
	}
}

func (r *Runner) dialNotificationWebsocket(ctx context.Context, client *codersdk.Client, logger slog.Logger) (*websocket.Conn, error) {
	u, err := client.URL.Parse("/api/v2/notifications/inbox/watch")
	if err != nil {
		logger.Error(ctx, "parse notification URL", slog.Error(err))
		r.cfg.Metrics.AddError("parse_url")
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
		r.cfg.Metrics.AddError("dial")
		return nil, xerrors.Errorf("dial notification websocket: %w", err)
	}

	return conn, nil
}

// watchNotifications reads notifications from the websocket and returns error or nil
// once all expected notifications are received.
func (r *Runner) watchNotifications(ctx context.Context, conn *websocket.Conn, user codersdk.User, logger slog.Logger, expectedNotifications map[uuid.UUID]struct{}) error {
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
			r.cfg.Metrics.AddError("read_notification_websocket")
			return xerrors.Errorf("read notification: %w", err)
		}

		templateID := notif.Notification.TemplateID
		if _, exists := expectedNotifications[templateID]; exists {
			if _, received := receivedNotifications[templateID]; !received {
				receiptTime := time.Now()
				r.websocketReceiptTimesMu.Lock()
				r.websocketReceiptTimes[templateID] = receiptTime
				r.websocketReceiptTimesMu.Unlock()
				receivedNotifications[templateID] = struct{}{}

				logger.Info(ctx, "received expected notification",
					slog.F("template_id", templateID),
					slog.F("title", notif.Notification.Title),
					slog.F("receipt_time", receiptTime))
			}
		} else {
			logger.Debug(ctx, "received notification not being tested",
				slog.F("template_id", templateID),
				slog.F("title", notif.Notification.Title))
		}
	}
}

// watchNotificationsSMTP polls the SMTP HTTP API for notifications and returns error or nil
// once all expected notifications are received.
func (r *Runner) watchNotificationsSMTP(ctx context.Context, user codersdk.User, logger slog.Logger, expectedNotifications map[uuid.UUID]struct{}) error {
	logger.Info(ctx, "polling SMTP API for notifications",
		slog.F("email", user.Email),
		slog.F("expected_count", len(expectedNotifications)),
	)
	receivedNotifications := make(map[uuid.UUID]struct{})

	apiURL := fmt.Sprintf("%s/messages?email=%s", r.cfg.SMTPApiURL, user.Email)
	httpClient := r.cfg.SMTPHttpClient

	const smtpPollInterval = 2 * time.Second
	done := xerrors.New("done")

	tkr := r.clock.TickerFunc(ctx, smtpPollInterval, func() error {
		reqCtx, cancel := context.WithTimeout(ctx, r.cfg.SMTPRequestTimeout)
		defer cancel()

		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, apiURL, nil)
		if err != nil {
			logger.Error(ctx, "create SMTP API request", slog.Error(err))
			r.cfg.Metrics.AddError("smtp_create_request")
			return xerrors.Errorf("create SMTP API request: %w", err)
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			logger.Error(ctx, "poll smtp api for notifications", slog.Error(err))
			r.cfg.Metrics.AddError("smtp_poll")
			return nil
		}

		if resp.StatusCode != http.StatusOK {
			// discard the response to allow reusing of the connection
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			logger.Error(ctx, "smtp api returned non-200 status", slog.F("status", resp.StatusCode))
			r.cfg.Metrics.AddError("smtp_bad_status")
			return nil
		}

		var summaries []smtpmock.EmailSummary
		if err := json.NewDecoder(resp.Body).Decode(&summaries); err != nil {
			_ = resp.Body.Close()
			logger.Error(ctx, "decode smtp api response", slog.Error(err))
			r.cfg.Metrics.AddError("smtp_decode")
			return xerrors.Errorf("decode smtp api response: %w", err)
		}
		_ = resp.Body.Close()

		// Process each email summary
		for _, summary := range summaries {
			notificationID := summary.AlertTemplateID
			if notificationID == uuid.Nil {
				continue
			}

			if _, exists := expectedNotifications[notificationID]; exists {
				if _, received := receivedNotifications[notificationID]; !received {
					receiptTime := summary.Date
					if receiptTime.IsZero() {
						receiptTime = time.Now()
					}

					r.smtpReceiptTimesMu.Lock()
					r.smtpReceiptTimes[notificationID] = receiptTime
					r.smtpReceiptTimesMu.Unlock()
					receivedNotifications[notificationID] = struct{}{}

					logger.Info(ctx, "received expected notification via SMTP",
						slog.F("notification_id", notificationID),
						slog.F("subject", summary.Subject),
						slog.F("receipt_time", receiptTime))
				}
			}
		}

		if len(receivedNotifications) == len(expectedNotifications) {
			logger.Info(ctx, "received all expected notifications via SMTP")
			return done
		}

		return nil
	}, "smtp")

	err := tkr.Wait()
	if errors.Is(err, done) {
		return nil
	}

	return err
}

func readNotification(ctx context.Context, conn *websocket.Conn) (codersdk.GetInboxAlertResponse, error) {
	_, message, err := conn.Read(ctx)
	if err != nil {
		return codersdk.GetInboxAlertResponse{}, err
	}

	var notif codersdk.GetInboxAlertResponse
	if err := json.Unmarshal(message, &notif); err != nil {
		return codersdk.GetInboxAlertResponse{}, xerrors.Errorf("unmarshal notification: %w", err)
	}

	return notif, nil
}
