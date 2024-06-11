package notifications

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/notifications/render"
	"github.com/coder/coder/v2/codersdk"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/notifications/dispatch"
	"github.com/coder/coder/v2/coderd/notifications/types"

	"cdr.dev/slog"
)

// Manager manages all notifications being enqueued and dispatched.
//
// Manager maintains a group of notifiers: these consume the queue of notification messages in the store.
//
// Notifiers dequeue messages from the store _CODER_NOTIFICATIONS_LEASE_COUNT_ at a time and concurrently "dispatch" these messages, meaning they are
// sent by their respective methods (email, webhook, etc).
//
// To reduce load on the store, successful and failed dispatches are accumulated in two separate buffers (success/failure)
// of size CODER_NOTIFICATIONS_STORE_SYNC_INTERVAL in the Manager, and updates are sent to the store about which messages
// succeeded or failed every CODER_NOTIFICATIONS_STORE_SYNC_INTERVAL seconds.
// These buffers are limited in size, and naturally introduce some backpressure; if there are hundreds of messages to be
// sent but they start failing too quickly, the buffers (receive channels) will fill up and block senders, which will
// slow down the dispatch rate.
//
// NOTE: The above backpressure mechanism only works if all notifiers live within the same process, which may not be true
// forever, such as if we split notifiers out into separate targets for greater processing throughput; in this case we
// will need an alternative mechanism for handling backpressure.
type Manager struct {
	cfg codersdk.NotificationsConfig
	// TODO: expand this to allow for each notification to have custom delivery methods, or multiple, or none.
	// 		 For example, Larry might want email notifications for "workspace deleted" notifications, but Harry wants
	//		 Slack notifications, and Mary doesn't want any.
	method database.NotificationMethod

	store Store
	log   slog.Logger

	notifiers  []*notifier
	notifierMu sync.Mutex

	handlers *HandlerRegistry
	helpers  map[string]any

	stopOnce sync.Once
	stop     chan any
	done     chan any
}

// NewManager instantiates a new Manager instance which coordinates notification enqueuing and delivery.
//
// helpers is a map of template helpers which are used to customize notification messages to use global settings like
// access URL etc.
func NewManager(cfg codersdk.NotificationsConfig, store Store, log slog.Logger, helpers map[string]any) (*Manager, error) {
	var method database.NotificationMethod
	if err := method.Scan(cfg.Method.String()); err != nil {
		return nil, xerrors.Errorf("given notification method %q is invalid", cfg.Method)
	}

	return &Manager{
		log:    log,
		cfg:    cfg,
		store:  store,
		method: method,

		stop: make(chan any),
		done: make(chan any),

		handlers: defaultHandlers(cfg, log),
		helpers:  helpers,
	}, nil
}

// defaultHandlers builds a set of known handlers; panics if any error occurs as these handlers should be valid at compile time.
func defaultHandlers(cfg codersdk.NotificationsConfig, log slog.Logger) *HandlerRegistry {
	reg, err := NewHandlerRegistry(
		dispatch.NewSMTPHandler(cfg.SMTP, log.Named("dispatcher.smtp")),
		dispatch.NewWebhookHandler(cfg.Webhook, log.Named("dispatcher.webhook")),
	)
	if err != nil {
		panic(err)
	}
	return reg
}

// WithHandlers allows for tests to inject their own handlers to verify functionality.
func (m *Manager) WithHandlers(reg *HandlerRegistry) {
	m.handlers = reg
}

// Run initiates the control loop in the background, which spawns a given number of notifier goroutines.
// Manager requires system-level permissions to interact with the store.
func (m *Manager) Run(ctx context.Context, notifiers int) {
	// Closes when Stop() is called or context is canceled.
	go func() {
		err := m.loop(ctx, notifiers)
		if err != nil {
			m.log.Error(ctx, "notification manager stopped with error", slog.Error(err))
		}
	}()
}

// loop contains the main business logic of the notification manager. It is responsible for subscribing to notification
// events, creating notifiers, and publishing bulk dispatch result updates to the store.
func (m *Manager) loop(ctx context.Context, notifiers int) error {
	defer func() {
		close(m.done)
		m.log.Info(context.Background(), "notification manager stopped")
	}()

	// Caught a terminal signal before notifiers were created, exit immediately.
	select {
	case <-m.stop:
		m.log.Warn(ctx, "gracefully stopped")
		return xerrors.Errorf("gracefully stopped")
	case <-ctx.Done():
		m.log.Error(ctx, "ungracefully stopped", slog.Error(ctx.Err()))
		return xerrors.Errorf("notifications: %w", ctx.Err())
	default:
	}

	var (
		// Buffer successful/failed notification dispatches in memory to reduce load on the store.
		//
		// We keep separate buffered for success/failure right now because the bulk updates are already a bit janky,
		// see BulkMarkNotificationMessagesSent/BulkMarkNotificationMessagesFailed. If we had the ability to batch updates,
		// like is offered in https://docs.sqlc.dev/en/stable/reference/query-annotations.html#batchmany, we'd have a cleaner
		// approach to this - but for now this will work fine.
		success = make(chan dispatchResult, m.cfg.StoreSyncBufferSize)
		failure = make(chan dispatchResult, m.cfg.StoreSyncBufferSize)
	)

	// Create a specific number of notifiers to run concurrently.
	var eg errgroup.Group
	for i := 0; i < notifiers; i++ {
		eg.Go(func() error {
			m.notifierMu.Lock()
			n := newNotifier(ctx, m.cfg, i+1, m.log, m.store, m.handlers)
			m.notifiers = append(m.notifiers, n)
			m.notifierMu.Unlock()
			return n.run(ctx, success, failure)
		})
	}

	eg.Go(func() error {
		// Every interval, collect the messages in the channels and bulk update them in the database.
		tick := time.NewTicker(m.cfg.StoreSyncInterval.Value())
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				// Nothing we can do in this scenario except bail out; after the message lease expires, the messages will
				// be requeued and users will receive duplicates.
				// This is an explicit trade-off between keeping the database load light (by bulk-updating records) and
				// exactly-once delivery.
				//
				// The current assumption is that duplicate delivery of these messages is, at worst, slightly annoying.
				// If these notifications are triggering external actions (e.g. via webhooks) this could be more
				// consequential, and we may need a more sophisticated mechanism.
				//
				// TODO: mention the above tradeoff in documentation.
				m.log.Warn(ctx, "exiting ungracefully", slog.Error(ctx.Err()))

				if len(success)+len(failure) > 0 {
					m.log.Warn(ctx, "content canceled with pending updates in buffer, these messages will be sent again after lease expires",
						slog.F("success_count", len(success)), slog.F("failure_count", len(failure)))
				}
				return ctx.Err()
			case <-m.stop:
				if len(success)+len(failure) > 0 {
					m.log.Warn(ctx, "flushing buffered updates before stop",
						slog.F("success_count", len(success)), slog.F("failure_count", len(failure)))
					m.bulkUpdate(ctx, success, failure)
					m.log.Warn(ctx, "flushing updates done")
				}
				return nil
			case <-tick.C:
				m.bulkUpdate(ctx, success, failure)
			}
		}
	})

	err := eg.Wait()
	if err != nil {
		m.log.Error(ctx, "manager loop exited with error", slog.Error(err))
	}
	return err
}

// Enqueue queues a notification message for later delivery.
// Messages will be dequeued by a notifier later and dispatched.
func (m *Manager) Enqueue(ctx context.Context, userID, templateID uuid.UUID, labels types.Labels, createdBy string, targets ...uuid.UUID) (*uuid.UUID, error) {
	payload, err := m.buildPayload(ctx, userID, templateID, labels)
	if err != nil {
		m.log.Warn(ctx, "failed to build payload", slog.F("template_id", templateID), slog.F("user_id", userID), slog.Error(err))
		return nil, xerrors.Errorf("enqueue notification (payload build): %w", err)
	}

	input, err := json.Marshal(payload)
	if err != nil {
		return nil, xerrors.Errorf("failed encoding input labels: %w", err)
	}

	id := uuid.New()
	msg, err := m.store.EnqueueNotificationMessage(ctx, database.EnqueueNotificationMessageParams{
		ID:                     id,
		UserID:                 userID,
		NotificationTemplateID: templateID,
		Method:                 m.method,
		Payload:                input,
		Targets:                targets,
		CreatedBy:              createdBy,
	})
	if err != nil {
		m.log.Warn(ctx, "failed to enqueue notification", slog.F("template_id", templateID), slog.F("input", input), slog.Error(err))
		return nil, xerrors.Errorf("enqueue notification: %w", err)
	}

	m.log.Debug(ctx, "enqueued notification", slog.F("msg_id", msg.ID))
	return &id, nil
}

// buildPayload creates the payload that the notification will for variable substitution and/or routing.
// The payload contains information about the recipient, the event that triggered the notification, and any subsequent
// actions which can be taken by the recipient.
func (m *Manager) buildPayload(ctx context.Context, userID uuid.UUID, templateID uuid.UUID, labels types.Labels) (*types.MessagePayload, error) {
	metadata, err := m.store.FetchNewMessageMetadata(ctx, database.FetchNewMessageMetadataParams{
		UserID:                 userID,
		NotificationTemplateID: templateID,
	})
	if err != nil {
		return nil, xerrors.Errorf("new message metadata: %w", err)
	}

	// Execute any templates in actions.
	out, err := render.GoTemplate(string(metadata.Actions), types.MessagePayload{}, m.helpers)
	if err != nil {
		return nil, xerrors.Errorf("render actions: %w", err)
	}
	metadata.Actions = []byte(out)

	var actions []types.TemplateAction
	if err = json.Unmarshal(metadata.Actions, &actions); err != nil {
		return nil, xerrors.Errorf("new message metadata: parse template actions: %w", err)
	}

	return &types.MessagePayload{
		Version: "1.0",

		NotificationName: metadata.NotificationName,

		UserID:    metadata.UserID.String(),
		UserEmail: metadata.UserEmail,
		UserName:  metadata.UserName,

		Actions: actions,
		Labels:  labels,
	}, nil
}

// bulkUpdate updates messages in the store based on the given successful and failed message dispatch results.
func (m *Manager) bulkUpdate(ctx context.Context, success, failure <-chan dispatchResult) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	nSuccess := len(success)
	nFailure := len(failure)

	// Nothing to do.
	if nSuccess+nFailure == 0 {
		return
	}

	var (
		successParams database.BulkMarkNotificationMessagesSentParams
		failureParams database.BulkMarkNotificationMessagesFailedParams
	)

	// Read all the existing messages due for update from the channel, but don't range over the channels because they
	// block until they are closed.
	//
	// This is vulnerable to TOCTOU, but it's fine.
	// If more items are added to the success or failure channels between measuring their lengths and now, those items
	// will be processed on the next bulk update.

	for i := 0; i < nSuccess; i++ {
		res := <-success
		successParams.IDs = append(successParams.IDs, res.msg)
		successParams.SentAts = append(successParams.SentAts, res.ts)
	}
	for i := 0; i < nFailure; i++ {
		res := <-failure

		status := database.NotificationMessageStatusPermanentFailure
		if res.retryable {
			status = database.NotificationMessageStatusTemporaryFailure
		}

		failureParams.IDs = append(failureParams.IDs, res.msg)
		failureParams.FailedAts = append(failureParams.FailedAts, res.ts)
		failureParams.Statuses = append(failureParams.Statuses, status)
		var reason string
		if res.err != nil {
			reason = res.err.Error()
		}
		failureParams.StatusReasons = append(failureParams.StatusReasons, reason)
	}

	// Execute bulk updates for success/failure concurrently.
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if len(successParams.IDs) == 0 {
			return
		}

		logger := m.log.With(slog.F("type", "update_sent"))

		// Give up after waiting for the store for 30s.
		uctx, cancel := context.WithTimeout(ctx, time.Second*30)
		defer cancel()

		n, err := m.store.BulkMarkNotificationMessagesSent(uctx, successParams)
		if err != nil {
			logger.Error(ctx, "bulk update failed", slog.Error(err))
			return
		}

		logger.Debug(ctx, "bulk update completed", slog.F("updated", n))
	}()

	go func() {
		defer wg.Done()
		if len(failureParams.IDs) == 0 {
			return
		}

		logger := m.log.With(slog.F("type", "update_failed"))

		// Give up after waiting for the store for 30s.
		uctx, cancel := context.WithTimeout(ctx, time.Second*30)
		defer cancel()

		failureParams.MaxAttempts = int32(m.cfg.MaxSendAttempts)
		failureParams.RetryInterval = int32(m.cfg.RetryInterval.Value().Seconds())
		n, err := m.store.BulkMarkNotificationMessagesFailed(uctx, failureParams)
		if err != nil {
			logger.Error(ctx, "bulk update failed", slog.Error(err))
			return
		}

		logger.Debug(ctx, "bulk update completed", slog.F("updated", n))
	}()

	wg.Wait()
}

// Stop stops all notifiers and waits until they have stopped.
func (m *Manager) Stop(ctx context.Context) error {
	var err error
	m.stopOnce.Do(func() {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			return
		default:
		}

		m.log.Info(context.Background(), "graceful stop requested")

		// If the notifiers haven't been started, we don't need to wait for anything.
		// This is only really during testing when we want to enqueue messages only but not deliver them.
		if len(m.notifiers) == 0 {
			close(m.done)
		}

		// Stop all notifiers.
		var eg errgroup.Group
		for _, n := range m.notifiers {
			eg.Go(func() error {
				n.stop()
				return nil
			})
		}
		_ = eg.Wait()

		// Signal the stop channel to cause loop to exit.
		close(m.stop)

		// Wait for the manager loop to exit or the context to be canceled, whichever comes first.
		select {
		case <-ctx.Done():
			var errStr string
			if ctx.Err() != nil {
				errStr = ctx.Err().Error()
			}
			// For some reason, slog.Error returns {} for a context error.
			m.log.Error(context.Background(), "graceful stop failed", slog.F("err", errStr))
			err = ctx.Err()
			return
		case <-m.done:
			m.log.Info(context.Background(), "gracefully stopped")
			return
		}
	})

	return err
}

type dispatchResult struct {
	notifier  int
	msg       uuid.UUID
	ts        time.Time
	err       error
	retryable bool
}

func newSuccessfulDispatch(notifier int, msg uuid.UUID) dispatchResult {
	return dispatchResult{
		notifier: notifier,
		msg:      msg,
		ts:       time.Now(),
	}
}

func newFailedDispatch(notifier int, msg uuid.UUID, err error, retryable bool) dispatchResult {
	return dispatchResult{
		notifier:  notifier,
		msg:       msg,
		ts:        time.Now(),
		err:       err,
		retryable: retryable,
	}
}
