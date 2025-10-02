package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/createusers"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/websocket"
)

type Runner struct {
	client *codersdk.Client
	cfg    Config

	createUserRunner *createusers.Runner

	userCreatedNotificationLatency time.Duration
	userDeletedNotificationLatency time.Duration
}

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	return &Runner{
		client: client,
		cfg:    cfg,
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

	logger.Info(ctx, "user created", slog.F("username", newUser.Username), slog.F("user_id", newUser.ID.String()))

	if r.cfg.IsOwner {
		logger.Info(ctx, "assigning Owner role to user")

		_, err := r.client.UpdateUserRoles(ctx, newUser.ID.String(), codersdk.UpdateRoles{
			Roles: []string{codersdk.RoleOwner},
		})
		if err != nil {
			r.cfg.Metrics.AddError(newUser.Username, "assign_owner_role")
			return xerrors.Errorf("assign owner role: %w", err)
		}
	}

	logger.Info(ctx, "notification runner is ready")

	// We don't need to wait for notifications since we're not an owner
	if !r.cfg.IsOwner {
		reachedBarrier = true
		r.cfg.DialBarrier.Done()
		return nil
	}

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

	logger.Info(ctx, "waiting for notifications", slog.F("timeout", r.cfg.NotificationTimeout))

	watchCtx, cancel := context.WithTimeout(ctx, r.cfg.NotificationTimeout)
	defer cancel()

	if err := r.watchNotifications(watchCtx, conn, newUser, logger); err != nil {
		return xerrors.Errorf("notification watch failed: %w", err)
	}

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
	UserCreatedNotificationLatencyMetric = "user_created_notification_latency_seconds"
	UserDeletedNotificationLatencyMetric = "user_deleted_notification_latency_seconds"
)

func (r *Runner) GetMetrics() map[string]any {
	metrics := map[string]any{}

	if r.userCreatedNotificationLatency > 0 {
		metrics[UserCreatedNotificationLatencyMetric] = r.userCreatedNotificationLatency.Seconds()
	}

	if r.userDeletedNotificationLatency > 0 {
		metrics[UserDeletedNotificationLatencyMetric] = r.userDeletedNotificationLatency.Seconds()
	}

	return metrics
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

// watchNotifications reads notifications from the websockert and returns error or nil
// once both expected notifications are received.
func (r *Runner) watchNotifications(ctx context.Context, conn *websocket.Conn, user codersdk.User, logger slog.Logger) error {
	notificationStartTime := time.Now()
	logger.Info(ctx, "waiting for notifications", slog.F("username", user.Username))

	receivedCreated := false
	receivedDeleted := false

	// Read notifications until we have both expected types
	for !receivedCreated || !receivedDeleted {
		notif, err := readNotification(ctx, conn)
		if err != nil {
			logger.Error(ctx, "read notification", slog.Error(err))
			r.cfg.Metrics.AddError(user.Username, "read_notification")
			return xerrors.Errorf("read notification: %w", err)
		}

		switch notif.Notification.TemplateID {
		case notifications.TemplateUserAccountCreated:
			if !receivedCreated {
				r.userCreatedNotificationLatency = time.Since(notificationStartTime)
				r.cfg.Metrics.RecordLatency(r.userCreatedNotificationLatency, user.Username, "user_created")
				receivedCreated = true
				logger.Info(ctx, "received user created notification")
			}
		case notifications.TemplateUserAccountDeleted:
			if !receivedDeleted {
				r.userDeletedNotificationLatency = time.Since(notificationStartTime)
				r.cfg.Metrics.RecordLatency(r.userDeletedNotificationLatency, user.Username, "user_deleted")
				receivedDeleted = true
				logger.Info(ctx, "received user deleted notification")
			}
		default:
			logger.Warn(ctx, "received unexpected notification type",
				slog.F("template_id", notif.Notification.TemplateID),
				slog.F("title", notif.Notification.Title))
		}
	}
	logger.Info(ctx, "received both notifications successfully")
	return nil
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
