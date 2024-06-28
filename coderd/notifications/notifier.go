package notifications

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/notifications/dispatch"
	"github.com/coder/coder/v2/coderd/notifications/render"
	"github.com/coder/coder/v2/coderd/notifications/types"
	"github.com/coder/coder/v2/codersdk"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
)

// notifier is a consumer of the notifications_messages queue. It dequeues messages from that table and processes them
// through a pipeline of fetch -> prepare -> render -> acquire handler -> deliver.
type notifier struct {
	id    uuid.UUID
	cfg   codersdk.NotificationsConfig
	log   slog.Logger
	store Store

	tick     *time.Ticker
	stopOnce sync.Once
	quit     chan any
	done     chan any

	handlers map[database.NotificationMethod]Handler
}

func newNotifier(cfg codersdk.NotificationsConfig, id uuid.UUID, log slog.Logger, db Store, hr map[database.NotificationMethod]Handler) *notifier {
	return &notifier{
		id:       id,
		cfg:      cfg,
		log:      log.Named("notifier").With(slog.F("notifier_id", id)),
		quit:     make(chan any),
		done:     make(chan any),
		tick:     time.NewTicker(cfg.FetchInterval.Value()),
		store:    db,
		handlers: hr,
	}
}

// run is the main loop of the notifier.
func (n *notifier) run(ctx context.Context, success chan<- dispatchResult, failure chan<- dispatchResult) error {
	defer func() {
		close(n.done)
		n.log.Info(context.Background(), "gracefully stopped")
	}()

	// TODO: idea from Cian: instead of querying the database on a short interval, we could wait for pubsub notifications.
	//		 if 100 notifications are enqueued, we shouldn't activate this routine for each one; so how to debounce these?
	//		 PLUS we should also have an interval (but a longer one, maybe 1m) to account for retries (those will not get
	//		 triggered by a code path, but rather by a timeout expiring which makes the message retryable)
	for {
		select {
		case <-ctx.Done():
			return xerrors.Errorf("notifier %q context canceled: %w", n.id, ctx.Err())
		case <-n.quit:
			return nil
		default:
		}

		// Call process() immediately (i.e. don't wait an initial tick).
		err := n.process(ctx, success, failure)
		if err != nil {
			n.log.Error(ctx, "failed to process messages", slog.Error(err))
		}

		// Shortcut to bail out quickly if stop() has been called or the context canceled.
		select {
		case <-ctx.Done():
			return xerrors.Errorf("notifier %q context canceled: %w", n.id, ctx.Err())
		case <-n.quit:
			return nil
		case <-n.tick.C:
			// sleep until next invocation
		}
	}
}

// process is responsible for coordinating the retrieval, processing, and delivery of messages.
// Messages are dispatched concurrently, but they may block when success/failure channels are full.
//
// NOTE: it is _possible_ that these goroutines could block for long enough to exceed CODER_NOTIFICATIONS_DISPATCH_TIMEOUT,
// resulting in a failed attempt for each notification when their contexts are canceled; this is not possible with the
// default configurations but could be brought about by an operator tuning things incorrectly.
func (n *notifier) process(ctx context.Context, success chan<- dispatchResult, failure chan<- dispatchResult) error {
	n.log.Debug(ctx, "attempting to dequeue messages")

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
		// A message failing to be prepared correctly should not affect other messages.
		deliverFn, err := n.prepare(ctx, msg)
		if err != nil {
			n.log.Warn(ctx, "dispatcher construction failed", slog.F("msg_id", msg.ID), slog.Error(err))
			failure <- newFailedDispatch(n.id, msg.ID, err, false)
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

	n.log.Debug(ctx, "dispatch completed", slog.F("count", len(msgs)))
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

	var title, body string
	if title, err = render.GoTemplate(msg.TitleTemplate, payload, nil); err != nil {
		return nil, xerrors.Errorf("render title: %w", err)
	}
	if body, err = render.GoTemplate(msg.BodyTemplate, payload, nil); err != nil {
		return nil, xerrors.Errorf("render body: %w", err)
	}

	return handler.Dispatcher(payload, title, body)
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
	logger := n.log.With(slog.F("msg_id", msg.ID), slog.F("method", msg.Method))

	retryable, err := deliver(ctx, msg.ID)
	if err != nil {
		// Don't try to accumulate message responses if the context has been canceled.
		//
		// This message's lease will expire in the store and will be requeued.
		// It's possible this will lead to a message being delivered more than once, and that is why Stop() is preferable
		// instead of canceling the context.
		//
		// In the case of backpressure (i.e. the success/failure channels are full because the database is slow),
		// and this caused delivery timeout (CODER_NOTIFICATIONS_DISPATCH_TIMEOUT), we can't append any more updates to
		// the channels otherwise this, too, will block.
		if xerrors.Is(err, context.Canceled) || xerrors.Is(err, context.DeadlineExceeded) {
			return err
		}

		logger.Warn(ctx, "message dispatch failed", slog.Error(err))
		failure <- newFailedDispatch(n.id, msg.ID, err, retryable)
	} else {
		logger.Debug(ctx, "message dispatch succeeded")
		success <- newSuccessfulDispatch(n.id, msg.ID)
	}

	return nil
}

// stop stops the notifier from processing any new notifications.
// This is a graceful stop, so any in-flight notifications will be completed before the notifier stops.
// Once a notifier has stopped, it cannot be restarted.
func (n *notifier) stop() {
	n.stopOnce.Do(func() {
		n.log.Info(context.Background(), "graceful stop requested")

		n.tick.Stop()
		close(n.quit)
		<-n.done
	})
}
