package notifications

import (
	"context"
	"sync"
	"text/template"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/quartz"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/notifications/dispatch"
	"github.com/coder/coder/v2/codersdk"
)

var ErrInvalidDispatchTimeout = xerrors.New("dispatch timeout must be less than lease period")

// Manager manages all notifications being enqueued and dispatched.
//
// Manager maintains a notifier: this consumes the queue of notification messages in the store.
//
// The notifier dequeues messages from the store _CODER_NOTIFICATIONS_LEASE_COUNT_ at a time and concurrently "dispatches"
// these messages, meaning they are sent by their respective methods (email, webhook, etc).
//
// To reduce load on the store, successful and failed dispatches are accumulated in two separate buffers (success/failure)
// of size CODER_NOTIFICATIONS_STORE_SYNC_INTERVAL in the Manager, and updates are sent to the store about which messages
// succeeded or failed every CODER_NOTIFICATIONS_STORE_SYNC_INTERVAL seconds.
// These buffers are limited in size, and naturally introduce some backpressure; if there are hundreds of messages to be
// sent but they start failing too quickly, the buffers (receive channels) will fill up and block senders, which will
// slow down the dispatch rate.
//
// NOTE: The above backpressure mechanism only works within the same process, which may not be true forever, such as if
// we split notifiers out into separate targets for greater processing throughput; in this case we will need an
// alternative mechanism for handling backpressure.
type Manager struct {
	cfg codersdk.NotificationsConfig

	store Store
	log   slog.Logger

	notifier *notifier
	handlers map[database.NotificationMethod]Handler
	method   database.NotificationMethod
	helpers  template.FuncMap

	metrics *Metrics

	success, failure chan dispatchResult

	runOnce  sync.Once
	stopOnce sync.Once
	doneOnce sync.Once
	stop     chan any
	done     chan any

	// clock is for testing only
	clock quartz.Clock
}

type ManagerOption func(*Manager)

// WithTestClock is used in testing to set the quartz clock on the manager
func WithTestClock(clock quartz.Clock) ManagerOption {
	return func(m *Manager) {
		m.clock = clock
	}
}

// NewManager instantiates a new Manager instance which coordinates notification enqueuing and delivery.
//
// helpers is a map of template helpers which are used to customize notification messages to use global settings like
// access URL etc.
func NewManager(cfg codersdk.NotificationsConfig, store Store, ps pubsub.Pubsub, helpers template.FuncMap, metrics *Metrics, log slog.Logger, opts ...ManagerOption) (*Manager, error) {
	var method database.NotificationMethod
	if err := method.Scan(cfg.Method.String()); err != nil {
		return nil, xerrors.Errorf("notification method %q is invalid", cfg.Method)
	}

	// If dispatch timeout exceeds lease period, it is possible that messages can be delivered in duplicate because the
	// lease can expire before the notifier gives up on the dispatch, which results in the message becoming eligible for
	// being re-acquired.
	if cfg.DispatchTimeout.Value() >= cfg.LeasePeriod.Value() {
		return nil, ErrInvalidDispatchTimeout
	}

	m := &Manager{
		log:   log,
		cfg:   cfg,
		store: store,

		// Buffer successful/failed notification dispatches in memory to reduce load on the store.
		//
		// We keep separate buffered for success/failure right now because the bulk updates are already a bit janky,
		// see BulkMarkNotificationMessagesSent/BulkMarkNotificationMessagesFailed. If we had the ability to batch updates,
		// like is offered in https://docs.sqlc.dev/en/stable/reference/query-annotations.html#batchmany, we'd have a cleaner
		// approach to this - but for now this will work fine.
		success: make(chan dispatchResult, cfg.StoreSyncBufferSize),
		failure: make(chan dispatchResult, cfg.StoreSyncBufferSize),

		metrics: metrics,
		method:  method,

		stop: make(chan any),
		done: make(chan any),

		handlers: defaultHandlers(cfg, log, store, ps),
		helpers:  helpers,

		clock: quartz.NewReal(),
	}
	for _, o := range opts {
		o(m)
	}
	return m, nil
}

// defaultHandlers builds a set of known handlers; panics if any error occurs as these handlers should be valid at compile time.
func defaultHandlers(cfg codersdk.NotificationsConfig, log slog.Logger, store Store, ps pubsub.Pubsub) map[database.NotificationMethod]Handler {
	return map[database.NotificationMethod]Handler{
		database.NotificationMethodSmtp:    dispatch.NewSMTPHandler(cfg.SMTP, log.Named("dispatcher.smtp")),
		database.NotificationMethodWebhook: dispatch.NewWebhookHandler(cfg.Webhook, log.Named("dispatcher.webhook")),
		database.NotificationMethodInbox:   dispatch.NewInboxHandler(log.Named("dispatcher.inbox"), store, ps),
	}
}

// WithHandlers allows for tests to inject their own handlers to verify functionality.
func (m *Manager) WithHandlers(reg map[database.NotificationMethod]Handler) {
	m.handlers = reg
}

// Run initiates the control loop in the background, which spawns a given number of notifier goroutines.
// Manager requires system-level permissions to interact with the store.
// Run is only intended to be run once.
func (m *Manager) Run(ctx context.Context) {
	m.log.Info(ctx, "started")

	m.runOnce.Do(func() {
		// Closes when Stop() is called or context is canceled.
		go func() {
			m.notifier = newNotifier(ctx, m.cfg, uuid.New(), m.log, m.store, m.handlers, m.helpers, m.metrics, m.clock)
			err := m.loop(ctx)
			if err != nil {
				m.log.Error(ctx, "notification manager stopped with error", slog.Error(err))
			}
		}()
	})
}

// loop contains the main business logic of the notification manager. It is responsible for subscribing to notification
// events, creating a notifier, and publishing bulk dispatch result updates to the store.
func (m *Manager) loop(ctx context.Context) error {
	defer func() {
		m.doneOnce.Do(func() {
			close(m.done)
		})
		m.log.Info(context.Background(), "notification manager stopped")
	}()

	// Caught a terminal signal before notifier was created, exit immediately.
	select {
	case <-m.stop:
		m.log.Warn(ctx, "gracefully stopped")
		return xerrors.Errorf("gracefully stopped")
	case <-ctx.Done():
		m.log.Error(ctx, "ungracefully stopped", slog.Error(ctx.Err()))
		return xerrors.Errorf("notifications: %w", ctx.Err())
	default:
	}

	var eg errgroup.Group

	eg.Go(func() error {
		// run the notifier which will handle dequeueing and dispatching notifications.
		return m.notifier.run(m.success, m.failure)
	})

	// Periodically flush notification state changes to the store.
	eg.Go(func() error {
		// Every interval, collect the messages in the channels and bulk update them in the store.
		tick := m.clock.NewTicker(m.cfg.StoreSyncInterval.Value(), "Manager", "storeSync")
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

				if len(m.success)+len(m.failure) > 0 {
					m.log.Warn(ctx, "content canceled with pending updates in buffer, these messages will be sent again after lease expires",
						slog.F("success_count", len(m.success)), slog.F("failure_count", len(m.failure)))
				}
				return ctx.Err()
			case <-m.stop:
				if len(m.success)+len(m.failure) > 0 {
					m.log.Warn(ctx, "flushing buffered updates before stop",
						slog.F("success_count", len(m.success)), slog.F("failure_count", len(m.failure)))
					m.syncUpdates(ctx)
					m.log.Warn(ctx, "flushing updates done")
				}
				return nil
			case <-tick.C:
				m.syncUpdates(ctx)
			}
		}
	})

	err := eg.Wait()
	if err != nil {
		m.log.Error(ctx, "manager loop exited with error", slog.Error(err))
	}
	return err
}

// BufferedUpdatesCount returns the number of buffered updates which are currently waiting to be flushed to the store.
// The returned values are for success & failure, respectively.
func (m *Manager) BufferedUpdatesCount() (success int, failure int) {
	return len(m.success), len(m.failure)
}

// syncUpdates updates messages in the store based on the given successful and failed message dispatch results.
func (m *Manager) syncUpdates(ctx context.Context) {
	// Ensure we update the metrics to reflect the current state after each invocation.
	defer func() {
		m.metrics.PendingUpdates.Set(float64(len(m.success) + len(m.failure)))
	}()

	select {
	case <-ctx.Done():
		return
	default:
	}

	nSuccess := len(m.success)
	nFailure := len(m.failure)

	m.metrics.PendingUpdates.Set(float64(nSuccess + nFailure))

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
		res := <-m.success
		successParams.IDs = append(successParams.IDs, res.msg)
		successParams.SentAts = append(successParams.SentAts, res.ts)
	}
	for i := 0; i < nFailure; i++ {
		res := <-m.failure

		var (
			reason string
			status database.NotificationMessageStatus
		)

		switch {
		case res.retryable:
			status = database.NotificationMessageStatusTemporaryFailure
		case res.inhibited:
			status = database.NotificationMessageStatusInhibited
			reason = "disabled by user"
		default:
			status = database.NotificationMessageStatusPermanentFailure
		}

		failureParams.IDs = append(failureParams.IDs, res.msg)
		failureParams.FailedAts = append(failureParams.FailedAts, res.ts)
		failureParams.Statuses = append(failureParams.Statuses, status)
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
		m.metrics.SyncedUpdates.Add(float64(n))

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

		// #nosec G115 - Safe conversion for max send attempts which is expected to be within int32 range
		failureParams.MaxAttempts = int32(m.cfg.MaxSendAttempts)
		failureParams.RetryInterval = int32(m.cfg.RetryInterval.Value().Seconds())
		n, err := m.store.BulkMarkNotificationMessagesFailed(uctx, failureParams)
		if err != nil {
			logger.Error(ctx, "bulk update failed", slog.Error(err))
			return
		}
		m.metrics.SyncedUpdates.Add(float64(n))

		logger.Debug(ctx, "bulk update completed", slog.F("updated", n))
	}()

	wg.Wait()
}

// Stop stops the notifier and waits until it has stopped.
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

		// If the notifier hasn't been started, we don't need to wait for anything.
		// This is only really during testing when we want to enqueue messages only but not deliver them.
		if m.notifier == nil {
			m.doneOnce.Do(func() {
				close(m.done)
			})
		} else {
			m.notifier.stop()
		}

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
	notifier  uuid.UUID
	msg       uuid.UUID
	ts        time.Time
	err       error
	retryable bool
	inhibited bool
}
