package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/apiversion"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/notifications/render"
	"github.com/coder/coder/v2/coderd/notifications/types"
	"github.com/coder/coder/v2/codersdk"
)

type WebhookDispatcher struct {
	cfg codersdk.NotificationsWebhookConfig
	log slog.Logger

	cl *http.Client
}

type WebhookPayload struct {
	Version string               `json:"_version"`
	MsgID   uuid.UUID            `json:"msg_id"`
	Payload types.MessagePayload `json:"payload"`
	Title   string               `json:"title"`
	Body    string               `json:"body"`
}

var (
	PayloadVersion = apiversion.New(1, 0)
)

func NewWebhookDispatcher(cfg codersdk.NotificationsWebhookConfig, log slog.Logger) *WebhookDispatcher {
	return &WebhookDispatcher{cfg: cfg, log: log, cl: &http.Client{}}
}

func (*WebhookDispatcher) NotificationMethod() database.NotificationMethod {
	// TODO: don't use database types
	return database.NotificationMethodWebhook
}

func (w *WebhookDispatcher) Dispatcher(payload types.MessagePayload, titleTmpl, bodyTmpl string) (DeliveryFunc, error) {
	title, err := render.Plaintext(titleTmpl)
	if err != nil {
		return nil, xerrors.Errorf("render title: %w", err)
	}
	body, err := render.Plaintext(bodyTmpl)
	if err != nil {
		return nil, xerrors.Errorf("render body: %w", err)
	}

	return w.dispatch(payload, title, body, w.cfg.Endpoint.String()), nil
}

func (w *WebhookDispatcher) dispatch(msgPayload types.MessagePayload, title, body, endpoint string) DeliveryFunc {
	return func(ctx context.Context, msgID uuid.UUID) (retryable bool, err error) {
		// Prepare payload.
		payload := WebhookPayload{
			Version: PayloadVersion.String(),
			MsgID:   msgID,
			Title:   title,
			Body:    body,
			Payload: msgPayload,
		}
		m, err := json.Marshal(payload)
		if err != nil {
			return false, xerrors.Errorf("marshal payload: %v", err)
		}

		// Prepare request.
		// Outer context has a deadline (see CODER_NOTIFICATIONS_DISPATCH_TIMEOUT).
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(m))
		if err != nil {
			return false, xerrors.Errorf("create HTTP request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		// Send request.
		resp, err := w.cl.Do(req)
		if err != nil {
			return true, xerrors.Errorf("failed to send HTTP request: %v", err)
		}
		defer resp.Body.Close()

		// Handle response.
		if resp.StatusCode/100 > 2 {
			var limitedResp []byte
			_, err = io.LimitReader(resp.Body, 100).Read(limitedResp)
			if err != nil {
				return true, xerrors.Errorf("non-200 response (%d), read body: %w", resp.StatusCode, err)
			}
			return true, xerrors.Errorf("non-200 response (%d): %s", resp.StatusCode, body)
		}

		return false, nil
	}
}
