package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"text/template"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications/dispatch"
	"github.com/coder/coder/v2/coderd/notifications/render"
	"github.com/coder/coder/v2/coderd/notifications/types"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
)

const (
	notificationsDefaultLogoURL = "https://coder.com/coder-logo-horizontal.png"
	notificationsDefaultAppName = "Coder"
)

type decorateHelpersError struct {
	inner error
}

func (e decorateHelpersError) Error() string {
	return fmt.Sprintf("failed to decorate helpers: %s", e.inner.Error())
}

func (e decorateHelpersError) Unwrap() error {
	return e.inner
}

func (decorateHelpersError) Is(other error) bool {
	_, ok := other.(decorateHelpersError)
	return ok
}

// notifier is a consumer of the notifications_messages queue. It dequeues messages from that table and processes them
// through a pipeline of fetch -> prepare -> render -> acquire handler -> deliver.
type notifier struct {
	id    uuid.UUID
	cfg   codersdk.NotificationsConfig
	log   slog.Logger
	store Store

	stopOnce       sync.Once
	outerCtx       context.Context
	gracefulCtx    context.Context
	gracefulCancel context.CancelFunc
	done           chan any

	handlers map[database.NotificationMethod]Handler
	metrics  *Metrics
	helpers  template.FuncMap

	// clock is for testing
	clock quartz.Clock
}

func newNotifier(outerCtx context.Context, cfg codersdk.NotificationsConfig, id uuid.UUID, log slog.Logger, db Store,
	hr map[database.NotificationMethod]Handler, helpers template.FuncMap, metrics *Metrics, clock quartz.Clock,
) *notifier {
	gracefulCtx, gracefulCancel := context.WithCancel(outerCtx)
	return &notifier{
		id:             id,
		cfg:            cfg,
		log:            log.Named("notifier").With(slog.F("notifier_id", id)),
		outerCtx:       outerCtx,
		gracefulCtx:    gracefulCtx,
		gracefulCancel: gracefulCancel,
		done:           make(chan any),
		store:          db,
		handlers:       hr,
		helpers:        helpers,
		metrics:        metrics,
		clock:          clock,
	}
}

// run is the main loop of the notifier.
func (n *notifier) run(success chan<- dispatchResult, failure chan<- dispatchResult) error {
	n.log.Info(n.outerCtx, "started")

	defer func() {
		close(n.done)
		n.log.Info(context.Background(), "gracefully stopped")
	}()

	// TODO: idea from Cian: instead of querying the database on a short interval, we could wait for pubsub notifications.
	//		 if 100 notifications are enqueued, we shouldn't activate this routine for each one; so how to debounce these?
	//		 PLUS we should also have an interval (but a longer one, maybe 1m) to account for retries (those will not get
	//		 triggered by a code path, but rather by a timeout expiring which makes the message retryable)

	// run the ticker with the graceful context, so we stop fetching after stop() is called
	tick := n.clock.TickerFunc(n.gracefulCtx, n.cfg.FetchInterval.Value(), func() error {
		// Check if notifier is not paused.
		ok, err := n.ensureRunning(n.outerCtx)
		if err != nil {
			n.log.Warn(n.outerCtx, "failed to check notifier state", slog.Error(err))
		}

		if ok {
			err = n.process(n.outerCtx, success, failure)
			if err != nil {
				n.log.Error(n.outerCtx, "failed to process messages", slog.Error(err))
			}
		}
		// we don't return any errors because we don't want to kill the loop because of them.
		return nil
	}, "notifier", "fetchInterval")

	_ = tick.Wait()
	// only errors we can return are context errors.  Only return an error if the outer context
	// was canceled, not if we were gracefully stopped.
	if n.outerCtx.Err() != nil {
		return xerrors.Errorf("notifier %q context canceled: %w", n.id, n.outerCtx.Err())
	}
	return nil
}

// ensureRunning checks if notifier is not paused.
func (n *notifier) ensureRunning(ctx context.Context) (bool, error) {
	settingsJSON, err := n.store.GetNotificationsSettings(ctx)
	if err != nil {
		return false, xerrors.Errorf("get notifications settings: %w", err)
	}

	var settings codersdk.NotificationsSettings
	if len(settingsJSON) == 0 {
		return true, nil // settings.NotifierPaused is false by default
	}

	err = json.Unmarshal([]byte(settingsJSON), &settings)
	if err != nil {
		return false, xerrors.Errorf("unmarshal notifications settings")
	}

	if settings.NotifierPaused {
		n.log.Debug(ctx, "notifier is paused, notifications will not be delivered")
	}
	return !settings.NotifierPaused, nil
}

// process is responsible for coordinating the retrieval, processing, and delivery of messages.
// Messages are dispatched concurrently, but they may block when success/failure channels are full.
//
// NOTE: it is _possible_ that these goroutines could block for long enough to exceed CODER_NOTIFICATIONS_DISPATCH_TIMEOUT,
// resulting in a failed attempt for each notification when their contexts are canceled; this is not possible with the
// default configurations but could be brought about by an operator tuning things incorrectly.
func (n *notifier) process(ctx context.Context, success chan<- dispatchResult, failure chan<- dispatchResult) error {
	msgs, err := n.fetch(ctx)
	if err != nil {
		return xerrors.Errorf("fetch messages: %w", err)
	}

	n.log.Debug(ctx, "dequeued messages", slog.F("count", len(msgs)))

	if len(msgs) == 0 {
		return nil
	}

	var eg errgroup.Group
	for _, msg := range msgs {
		// If a notification template has been disabled by the user after a notification was enqueued, mark it as inhibited
		if msg.Disabled {
			failure <- n.newInhibitedDispatch(msg)
			continue
		}

		// A message failing to be prepared correctly should not affect other messages.
		deliverFn, err := n.prepare(ctx, msg)
		if err != nil {
			if database.IsQueryCanceledError(err) {
				n.log.Debug(ctx, "dispatcher construction canceled", slog.F("msg_id", msg.ID), slog.Error(err))
			} else {
				n.log.Error(ctx, "dispatcher construction failed", slog.F("msg_id", msg.ID), slog.Error(err))
			}
			failure <- n.newFailedDispatch(msg, err, errors.Is(err, decorateHelpersError{}))
			n.metrics.PendingUpdates.Set(float64(len(success) + len(failure)))
			continue
		}

		eg.Go(func() error {
			// Dispatch must only return an error for exceptional cases, NOT for failed messages.
			return n.deliver(ctx, msg, deliverFn, success, failure)
		})
	}

	if err = eg.Wait(); err != nil {
		n.log.Debug(ctx, "dispatch failed", slog.Error(err))
		return xerrors.Errorf("dispatch failed: %w", err)
	}

	n.log.Debug(ctx, "batch completed", slog.F("count", len(msgs)))
	return nil
}

// fetch retrieves messages from the queue by "acquiring a lease" whereby this notifier is the exclusive handler of these
// messages until they are dispatched - or until the lease expires (in exceptional cases).
func (n *notifier) fetch(ctx context.Context) ([]database.AcquireNotificationMessagesRow, error) {
	msgs, err := n.store.AcquireNotificationMessages(ctx, database.AcquireNotificationMessagesParams{
		Count:           int32(n.cfg.LeaseCount),
		MaxAttemptCount: int32(n.cfg.MaxSendAttempts),
		NotifierID:      n.id,
		LeaseSeconds:    int32(n.cfg.LeasePeriod.Value().Seconds()),
	})
	if err != nil {
		return nil, xerrors.Errorf("acquire messages: %w", err)
	}

	return msgs, nil
}

// prepare has two roles:
// 1. render the title & body templates
// 2. build a dispatcher from the given message, payload, and these templates - to be used for delivering the notification
func (n *notifier) prepare(ctx context.Context, msg database.AcquireNotificationMessagesRow) (dispatch.DeliveryFunc, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// NOTE: when we change the format of the MessagePayload, we have to bump its version and handle unmarshalling
	// differently here based on that version.
	var payload types.MessagePayload
	err := json.Unmarshal(msg.Payload, &payload)
	if err != nil {
		return nil, xerrors.Errorf("unmarshal payload: %w", err)
	}

	handler, ok := n.handlers[msg.Method]
	if !ok {
		return nil, xerrors.Errorf("failed to resolve handler %q", msg.Method)
	}

	helpers, err := n.fetchHelpers(ctx)
	if err != nil {
		return nil, decorateHelpersError{err}
	}

	var title, body string
	if title, err = render.GoTemplate(msg.TitleTemplate, payload, helpers); err != nil {
		return nil, xerrors.Errorf("render title: %w", err)
	}
	if body, err = render.GoTemplate(msg.BodyTemplate, payload, helpers); err != nil {
		return nil, xerrors.Errorf("render body: %w", err)
	}

	return handler.Dispatcher(payload, title, body, helpers)
}

// deliver sends a given notification message via its defined method.
// This method *only* returns an error when a context error occurs; any other error is interpreted as a failure to
// deliver the notification and as such the message will be marked as failed (to later be optionally retried).
func (n *notifier) deliver(ctx context.Context, msg database.AcquireNotificationMessagesRow, deliver dispatch.DeliveryFunc, success, failure chan<- dispatchResult) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	ctx, cancel := context.WithTimeout(ctx, n.cfg.DispatchTimeout.Value())
	defer cancel()
	logger := n.log.With(slog.F("msg_id", msg.ID), slog.F("method", msg.Method), slog.F("attempt", msg.AttemptCount+1))

	if msg.AttemptCount > 0 {
		n.metrics.RetryCount.WithLabelValues(string(msg.Method), msg.TemplateID.String()).Inc()
	}

	n.metrics.InflightDispatches.WithLabelValues(string(msg.Method), msg.TemplateID.String()).Inc()
	n.metrics.QueuedSeconds.WithLabelValues(string(msg.Method)).Observe(msg.QueuedSeconds)

	start := n.clock.Now()
	retryable, err := deliver(ctx, msg.ID)

	n.metrics.DispatcherSendSeconds.WithLabelValues(string(msg.Method)).Observe(n.clock.Since(start).Seconds())
	n.metrics.InflightDispatches.WithLabelValues(string(msg.Method), msg.TemplateID.String()).Dec()

	if err != nil {
		// Don't try to accumulate message responses if the context has been canceled.
		//
		// This message's lease will expire in the store and will be requeued.
		// It's possible this will lead to a message being delivered more than once, and that is why Stop() is preferable
		// instead of canceling the context.
		//
		// In the case of backpressure (i.e. the success/failure channels are full because the database is slow),
		// we can't append any more updates to the channels otherwise this, too, will block.
		if errors.Is(err, context.Canceled) {
			return err
		}

		select {
		case <-ctx.Done():
			logger.Warn(context.Background(), "cannot record dispatch failure result", slog.Error(ctx.Err()))
			return ctx.Err()
		case failure <- n.newFailedDispatch(msg, err, retryable):
			logger.Warn(ctx, "message dispatch failed", slog.Error(err))
		}
	} else {
		select {
		case <-ctx.Done():
			logger.Warn(context.Background(), "cannot record dispatch success result", slog.Error(ctx.Err()))
			return ctx.Err()
		case success <- n.newSuccessfulDispatch(msg):
			logger.Debug(ctx, "message dispatch succeeded")
		}
	}
	n.metrics.PendingUpdates.Set(float64(len(success) + len(failure)))

	return nil
}

func (n *notifier) newSuccessfulDispatch(msg database.AcquireNotificationMessagesRow) dispatchResult {
	n.metrics.DispatchAttempts.WithLabelValues(string(msg.Method), msg.TemplateID.String(), ResultSuccess).Inc()

	return dispatchResult{
		notifier: n.id,
		msg:      msg.ID,
		ts:       dbtime.Time(n.clock.Now().UTC()),
	}
}

// revive:disable-next-line:flag-parameter // Not used for control flow, rather just choosing which metric to increment.
func (n *notifier) newFailedDispatch(msg database.AcquireNotificationMessagesRow, err error, retryable bool) dispatchResult {
	var result string

	// If retryable and not the last attempt, it's a temporary failure.
	if retryable && msg.AttemptCount < int32(n.cfg.MaxSendAttempts)-1 {
		result = ResultTempFail
	} else {
		result = ResultPermFail
	}

	n.metrics.DispatchAttempts.WithLabelValues(string(msg.Method), msg.TemplateID.String(), result).Inc()

	return dispatchResult{
		notifier:  n.id,
		msg:       msg.ID,
		ts:        dbtime.Time(n.clock.Now().UTC()),
		err:       err,
		retryable: retryable,
	}
}

func (n *notifier) newInhibitedDispatch(msg database.AcquireNotificationMessagesRow) dispatchResult {
	return dispatchResult{
		notifier:  n.id,
		msg:       msg.ID,
		ts:        dbtime.Time(n.clock.Now().UTC()),
		retryable: false,
		inhibited: true,
	}
}

// stop stops the notifier from processing any new notifications.
// This is a graceful stop, so any in-flight notifications will be completed before the notifier stops.
// Once a notifier has stopped, it cannot be restarted.
func (n *notifier) stop() {
	n.stopOnce.Do(func() {
		n.log.Info(context.Background(), "graceful stop requested")
		n.gracefulCancel()
		<-n.done
	})
}
