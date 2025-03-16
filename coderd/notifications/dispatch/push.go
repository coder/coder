package dispatch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"text/template"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/notifications/types"
	markdown "github.com/coder/coder/v2/coderd/render"
	"github.com/coder/coder/v2/codersdk"
)

type PushStore interface {
	GetNotificationPushSubscriptionsByUserID(ctx context.Context, userID uuid.UUID) ([]database.NotificationPushSubscription, error)
	DeleteNotificationPushSubscriptions(ctx context.Context, subscriptionIDs []uuid.UUID) error
}

type PushHandler struct {
	log             slog.Logger
	store           PushStore
	vapidPublicKey  string
	vapidPrivateKey string
}

func NewPushHandler(cfg codersdk.NotificationsPushConfig, log slog.Logger, store PushStore) *PushHandler {
	return &PushHandler{log: log, store: store, vapidPublicKey: cfg.VAPIDPublicKey.String(), vapidPrivateKey: cfg.VAPIDPrivateKey.String()}
}

func (h *PushHandler) Dispatcher(payload types.MessagePayload, titleTmpl, bodyTmpl string, _ template.FuncMap) (DeliveryFunc, error) {
	subject, err := markdown.PlaintextFromMarkdown(titleTmpl)
	if err != nil {
		return nil, xerrors.Errorf("render subject: %w", err)
	}

	htmlBody, err := markdown.PlaintextFromMarkdown(bodyTmpl)
	if err != nil {
		return nil, xerrors.Errorf("render html body: %w", err)
	}

	return h.dispatch(payload, subject, htmlBody), nil
}

func (h *PushHandler) dispatch(payload types.MessagePayload, subject, htmlBody string) DeliveryFunc {
	return func(ctx context.Context, msgID uuid.UUID) (bool, error) {
		userID, err := uuid.Parse(payload.UserID)
		if err != nil {
			return false, xerrors.Errorf("parse user ID: %w", err)
		}
		subscriptions, err := h.store.GetNotificationPushSubscriptionsByUserID(ctx, userID)
		if err != nil {
			return false, xerrors.Errorf("get notification push subscriptions by user ID: %w", err)
		}

		actions := make([]codersdk.PushNotificationAction, len(payload.Actions))
		for i, action := range payload.Actions {
			actions[i] = codersdk.PushNotificationAction{
				Label: action.Label,
				URL:   action.URL,
			}
		}

		var icon string
		if payload.Data != nil {
			icon, _ = payload.Data["icon"].(string)
		}
		notification := codersdk.PushNotification{
			Icon:    icon,
			Title:   subject,
			Body:    htmlBody,
			Actions: actions,
		}
		notificationJSON, err := json.Marshal(notification)
		if err != nil {
			return false, xerrors.Errorf("marshal notification: %w", err)
		}

		cleanupSubscriptions := make([]uuid.UUID, 0)
		var mu sync.Mutex
		var eg errgroup.Group
		for _, subscription := range subscriptions {
			subscription := subscription
			eg.Go(func() error {
				h.log.Debug(ctx, "dispatching via push", slog.F("msg_id", msgID), slog.F("subscription", subscription.Endpoint))

				resp, err := webpush.SendNotificationWithContext(ctx, notificationJSON, &webpush.Subscription{
					Endpoint: subscription.Endpoint,
					Keys: webpush.Keys{
						Auth:   subscription.EndpointAuthKey,
						P256dh: subscription.EndpointP256dhKey,
					},
				}, &webpush.Options{
					VAPIDPublicKey:  h.vapidPublicKey,
					VAPIDPrivateKey: h.vapidPrivateKey,
				})
				if err != nil {
					return xerrors.Errorf("send notification: %w", err)
				}
				defer resp.Body.Close()

				if resp.StatusCode == http.StatusGone {
					// The subscription is no longer valid, remove it.
					mu.Lock()
					cleanupSubscriptions = append(cleanupSubscriptions, subscription.ID)
					mu.Unlock()
					return nil
				}

				// 200, 201, and 202 are common for successful delivery.
				if resp.StatusCode > http.StatusAccepted {
					// It's likely the subscription failed to deliver for some reason.
					body, _ := io.ReadAll(resp.Body)
					return xerrors.Errorf("push notification failed with status code %d: %s", resp.StatusCode, string(body))
				}

				return nil
			})
		}

		err = eg.Wait()
		if err != nil {
			return false, xerrors.Errorf("send push notifications: %w", err)
		}

		if len(cleanupSubscriptions) > 0 {
			// nolint:gocritic // These are known to be invalid subscriptions.
			err = h.store.DeleteNotificationPushSubscriptions(dbauthz.AsSystemRestricted(ctx), cleanupSubscriptions)
			if err != nil {
				h.log.Error(ctx, "failed to delete stale push subscriptions", slog.F("msg_id", msgID), slog.F("error", err))
			}
		}

		return false, nil
	}
}
