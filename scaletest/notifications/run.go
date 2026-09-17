package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	notificationsLib "github.com/coder/coder/v2/coderd/notifications"
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

	websocketDeletionReceiptTimes   []time.Time
	websocketDeletionReceiptTimesMu sync.RWMutex

	// smtpReceiptTimes holds receipt times for SMTP template-deletion emails,
	// deduped by message ID.
	smtpReceiptTimes   []time.Time
	smtpReceiptTimesMu sync.RWMutex

	clock quartz.Clock
}

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	return &Runner{
		client: client,
		cfg:    cfg,
		clock:  quartz.NewReal(),
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
		if r.cfg.IsTemplateAdmin && !reachedReceivingWatchBarrier {
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

	if !r.cfg.IsTemplateAdmin {
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
		return r.watchNotifications(egCtx, conn, newUser, logger, r.cfg.ExpectedDeletions)
	})

	if r.cfg.SMTPApiURL != "" {
		logger.Info(ctx, "running SMTP notification watcher")
		eg.Go(func() error {
			return r.watchNotificationsSMTP(egCtx, newUser, logger, r.cfg.ExpectedDeletions)
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
	r.websocketDeletionReceiptTimesMu.RLock()
	websocketDeletionReceiptTimes := slices.Clone(r.websocketDeletionReceiptTimes)
	r.websocketDeletionReceiptTimesMu.RUnlock()

	r.smtpReceiptTimesMu.RLock()
	smtpReceiptTimes := slices.Clone(r.smtpReceiptTimes)
	r.smtpReceiptTimesMu.RUnlock()

	return map[string]any{
		WebsocketNotificationReceiptTimeMetric: websocketDeletionReceiptTimes,
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

// watchNotifications reads notifications from the websocket and returns once
// expectedDeletions distinct TemplateTemplateDeleted notifications have been
// received. Deduplication is by notification instance ID so repeated deliveries
// of the same notification are counted once; any other notification type is
// ignored.
func (r *Runner) watchNotifications(ctx context.Context, conn *websocket.Conn, user codersdk.User, logger slog.Logger, expectedDeletions int) error {
	logger.Info(ctx, "waiting for notifications",
		slog.F("username", user.Username),
		slog.F("expected_deletions", expectedDeletions))

	// seen tracks notification instance IDs so repeated deliveries of the same
	// notification are counted once; len(seen) is the number counted so far.
	seen := make(map[uuid.UUID]bool)

	for len(seen) < expectedDeletions {
		select {
		case <-ctx.Done():
			return xerrors.Errorf("context canceled while waiting for notifications: %w", ctx.Err())
		default:
		}

		notif, err := readNotification(ctx, conn)
		if err != nil {
			logger.Error(ctx, "read notification", slog.Error(err))
			r.cfg.Metrics.AddError("read_notification_websocket")
			return xerrors.Errorf("read notification: %w", err)
		}

		// This load generator only triggers template deletions, so ignore any
		// other notification type.
		if notif.Notification.TemplateID != notificationsLib.TemplateTemplateDeleted {
			logger.Debug(ctx, "ignoring non-deletion notification",
				slog.F("template_id", notif.Notification.TemplateID),
				slog.F("title", notif.Notification.Title))
			continue
		}

		if seen[notif.Notification.ID] {
			continue
		}
		seen[notif.Notification.ID] = true

		receiptTime := time.Now()
		r.websocketDeletionReceiptTimesMu.Lock()
		r.websocketDeletionReceiptTimes = append(r.websocketDeletionReceiptTimes, receiptTime)
		r.websocketDeletionReceiptTimesMu.Unlock()

		logger.Info(ctx, "received expected notification",
			slog.F("notification_id", notif.Notification.ID),
			slog.F("count", len(seen)),
			slog.F("want", expectedDeletions),
			slog.F("title", notif.Notification.Title),
			slog.F("receipt_time", receiptTime))
	}

	logger.Info(ctx, "received all expected notifications")
	return nil
}

// watchNotificationsSMTP polls the SMTP HTTP API and returns once
// expectedDeletions distinct TemplateTemplateDeleted emails have been received.
// Deduplication is by message ID so repeated polls of the same email are counted
// once; any other notification type is ignored.
func (r *Runner) watchNotificationsSMTP(ctx context.Context, user codersdk.User, logger slog.Logger, expectedDeletions int) error {
	logger.Info(ctx, "polling SMTP API for notifications",
		slog.F("email", user.Email),
		slog.F("expected_deletions", expectedDeletions))

	// seen tracks message IDs so repeated polls of the same email are counted
	// once; len(seen) is the number counted so far.
	seen := make(map[string]bool)

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

		for _, summary := range summaries {
			// This load generator only triggers template deletions, so ignore any
			// other notification type.
			if summary.NotificationTemplateID != notificationsLib.TemplateTemplateDeleted {
				continue
			}
			if seen[summary.MessageID] {
				continue
			}
			seen[summary.MessageID] = true

			receiptTime := summary.Date
			if receiptTime.IsZero() {
				receiptTime = time.Now()
			}

			r.smtpReceiptTimesMu.Lock()
			r.smtpReceiptTimes = append(r.smtpReceiptTimes, receiptTime)
			r.smtpReceiptTimesMu.Unlock()

			logger.Info(ctx, "received expected notification via SMTP",
				slog.F("message_id", summary.MessageID),
				slog.F("subject", summary.Subject),
				slog.F("count", len(seen)),
				slog.F("want", expectedDeletions),
				slog.F("receipt_time", receiptTime))
		}

		if len(seen) >= expectedDeletions {
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
