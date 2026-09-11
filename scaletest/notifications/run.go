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
	client *codersdk.Client
	cfg    Config

	// websocketReceiptTimes stores the receipt times for websocket notifications,
	// keyed by notification template ID. A type may be received more than once
	// (for example one TemplateTemplateDeleted per template deletion), so every
	// receipt is recorded to produce one latency sample per notification.
	websocketReceiptTimes   map[uuid.UUID][]time.Time
	websocketReceiptTimesMu sync.RWMutex

	// smtpReceiptTimes stores the receipt times for SMTP notifications, keyed by
	// notification template ID.
	smtpReceiptTimes   map[uuid.UUID][]time.Time
	smtpReceiptTimesMu sync.RWMutex

	clock quartz.Clock
}

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	return &Runner{
		client:                client,
		cfg:                   cfg,
		websocketReceiptTimes: make(map[uuid.UUID][]time.Time),
		smtpReceiptTimes:      make(map[uuid.UUID][]time.Time),
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
		if len(r.cfg.ExpectedNotifications) > 0 && !reachedReceivingWatchBarrier {
			r.cfg.ReceivingWatchBarrier.Done()
		}
	}()

	logs = loadtestutil.NewSyncWriter(logs)
	logger := slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug)
	r.client.SetLogger(logger)
	r.client.SetLogBodies(true)

	// The runner always reuses an existing user; it never creates one. The user
	// must already hold any role needed to receive the notifications under test.
	newUser := r.cfg.PreCreatedUser
	newUserClient := codersdk.New(r.client.URL,
		codersdk.WithSessionToken(r.cfg.SessionToken),
		codersdk.WithLogger(logger),
		codersdk.WithLogBodies())

	logger.Info(ctx, "reusing existing user", slog.F("username", newUser.Username), slog.F("user_id", newUser.ID.String()))

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

func (*Runner) Cleanup(_ context.Context, _ string, _ io.Writer) error {
	// The runner reuses an existing user and never creates one, so there is
	// nothing to clean up.
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
// once all expected notifications are received. expectedNotifications maps a
// notification template ID to the number of notifications of that type to wait
// for; deduplication is by notification instance ID so repeated deliveries of
// the same notification are not double counted, while distinct notifications of
// the same type (for example multiple template deletions) are.
func (r *Runner) watchNotifications(ctx context.Context, conn *websocket.Conn, user codersdk.User, logger slog.Logger, expectedNotifications map[uuid.UUID]int) error {
	totalExpected := 0
	for _, count := range expectedNotifications {
		totalExpected += count
	}

	logger.Info(ctx, "waiting for notifications",
		slog.F("username", user.Username),
		slog.F("expected_count", totalExpected))

	// seen tracks notification instance IDs to guard against duplicate delivery.
	seen := make(map[uuid.UUID]struct{})
	// receivedCounts tracks how many notifications of each type have been counted.
	receivedCounts := make(map[uuid.UUID]int)

	complete := func() bool {
		for templateID, want := range expectedNotifications {
			if receivedCounts[templateID] < want {
				return false
			}
		}
		return true
	}

	for {
		select {
		case <-ctx.Done():
			return xerrors.Errorf("context canceled while waiting for notifications: %w", ctx.Err())
		default:
		}

		if complete() {
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
		want, expected := expectedNotifications[templateID]
		if !expected {
			logger.Debug(ctx, "received notification not being tested",
				slog.F("template_id", templateID),
				slog.F("title", notif.Notification.Title))
			continue
		}

		if _, dup := seen[notif.Notification.ID]; dup {
			continue
		}
		seen[notif.Notification.ID] = struct{}{}

		if receivedCounts[templateID] >= want {
			continue
		}

		receiptTime := time.Now()
		receivedCounts[templateID]++
		// Record every receipt so each delivered notification produces a latency
		// sample; a single type may arrive multiple times (one per deletion).
		r.websocketReceiptTimesMu.Lock()
		r.websocketReceiptTimes[templateID] = append(r.websocketReceiptTimes[templateID], receiptTime)
		r.websocketReceiptTimesMu.Unlock()

		logger.Info(ctx, "received expected notification",
			slog.F("template_id", templateID),
			slog.F("notification_id", notif.Notification.ID),
			slog.F("count", receivedCounts[templateID]),
			slog.F("want", want),
			slog.F("title", notif.Notification.Title),
			slog.F("receipt_time", receiptTime))
	}
}

// watchNotificationsSMTP polls the SMTP HTTP API for notifications and returns error or nil
// once all expected notifications are received. It waits for at least one email
// per expected notification type; SMTP summaries carry no per-message ID, so it
// cannot distinguish repeated deliveries of the same type.
func (r *Runner) watchNotificationsSMTP(ctx context.Context, user codersdk.User, logger slog.Logger, expectedNotifications map[uuid.UUID]int) error {
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
					receiptTime := summary.Date
					if receiptTime.IsZero() {
						receiptTime = time.Now()
					}

					r.smtpReceiptTimesMu.Lock()
					r.smtpReceiptTimes[notificationID] = append(r.smtpReceiptTimes[notificationID], receiptTime)
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
