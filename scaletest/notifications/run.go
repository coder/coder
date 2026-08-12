package notifications

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

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/coder/v2/scaletest/smtpmock"
	"github.com/coder/quartz"
	"github.com/coder/websocket"
)

type Runner struct {
	cfg Config

	// websocketReceiptTimes stores the receipt time for websocket notifications
	websocketReceiptTimes   map[uuid.UUID]time.Time
	websocketReceiptTimesMu sync.RWMutex

	// smtpReceiptTimes stores the receipt time for SMTP notifications
	smtpReceiptTimes   map[uuid.UUID]time.Time
	smtpReceiptTimesMu sync.RWMutex

	clock quartz.Clock
}

// NewRunner returns a runner that dials the notification websocket as
// cfg.PreCreatedUser. It needs no API client: the caller owns the user lifecycle,
// so the handshake is the only request a runner makes.
func NewRunner(cfg Config) *Runner {
	return &Runner{
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
	_ harness.Collectable = &Runner{}
)

func (r *Runner) Run(ctx context.Context, _ string, logs io.Writer) error {
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
		if len(r.cfg.ExpectedNotificationIDs) > 0 && !reachedReceivingWatchBarrier {
			r.cfg.ReceivingWatchBarrier.Done()
		}
	}()

	logs = loadtestutil.NewSyncWriter(logs)
	logger := slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug)

	// Config.Validate owns this contract; these are defensive guards against a
	// caller that bypasses validation. A caller-side programming error is not a
	// load-test failure, so neither is recorded as an error metric.
	if r.cfg.PreCreatedUser.ID == uuid.Nil {
		return xerrors.New("pre-created user required but not provided")
	}
	if r.cfg.SessionToken == "" {
		return xerrors.New("session token required but not provided")
	}
	user := r.cfg.PreCreatedUser
	userClient := codersdk.New(r.cfg.URL,
		codersdk.WithSessionToken(r.cfg.SessionToken),
		codersdk.WithLogger(logger),
		codersdk.WithLogBodies())
	// Dial with the caller's HTTP client so the handshake uses its TLS and proxy
	// configuration.
	userClient.HTTPClient = r.cfg.DialHTTPClient

	logger.Info(ctx, "notification runner is ready", slog.F("username", user.Username), slog.F("user_id", user.ID.String()))

	dialCtx, cancel := context.WithTimeout(ctx, r.cfg.DialTimeout)
	defer cancel()

	logger.Info(ctx, "connecting to notification websocket")
	conn, err := r.dialNotificationWebsocket(dialCtx, userClient, logger)
	if err != nil {
		return xerrors.Errorf("dial notification websocket: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")
	logger.Info(ctx, "connected to notification websocket")

	reachedBarrier = true
	r.cfg.DialBarrier.Done()
	r.cfg.DialBarrier.Wait()

	if len(r.cfg.ExpectedNotificationIDs) == 0 {
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

	// The watch runs until the caller's context expires. That context carries the
	// overall test budget, so there is no separate per-runner notification
	// deadline that could expire before or after it.
	if deadline, ok := ctx.Deadline(); ok {
		logger.Info(ctx, "waiting for notifications", slog.F("deadline", deadline))
	} else {
		logger.Info(ctx, "waiting for notifications")
	}

	eg, egCtx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		return r.watchNotifications(egCtx, conn, user, logger, r.cfg.ExpectedNotificationIDs)
	})

	if r.cfg.SMTPApiURL != "" {
		logger.Info(ctx, "running SMTP notification watcher")
		eg.Go(func() error {
			return r.watchNotificationsSMTP(egCtx, user, logger, r.cfg.ExpectedNotificationIDs)
		})
	}

	if err := eg.Wait(); err != nil {
		return xerrors.Errorf("notification watch failed: %w", err)
	}

	reachedReceivingWatchBarrier = true
	r.cfg.ReceivingWatchBarrier.Done()

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

// dialNotificationWebsocket connects to the inbox watch endpoint. Client.Dial
// parses the URL, propagates the client's HTTP client so custom TLS applies, and
// sets the session token header.
func (r *Runner) dialNotificationWebsocket(ctx context.Context, client *codersdk.Client, logger slog.Logger) (*websocket.Conn, error) {
	conn, err := client.Dial(ctx, "/api/v2/notifications/inbox/watch", nil)
	if err != nil {
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
			notificationID := summary.NotificationTemplateID
			if notificationID == uuid.Nil {
				continue
			}

			if _, exists := expectedNotifications[notificationID]; exists {
				if _, received := receivedNotifications[notificationID]; !received {
					// The SMTP mock stamps this date on its own host, while the trigger time
					// comes from the CLI host, so clock skew between the two lands in the
					// reported SMTP latency and can even make it negative. The websocket
					// measurement stays on one clock and does not have this problem.
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
