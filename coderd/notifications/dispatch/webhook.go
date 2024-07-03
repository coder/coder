package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/notifications/types"
	markdown "github.com/coder/coder/v2/coderd/render"
	"github.com/coder/coder/v2/codersdk"
)

// WebhookHandler dispatches notification messages via an HTTP POST webhook.
type WebhookHandler struct {
	cfg codersdk.NotificationsWebhookConfig
	log slog.Logger

	cl *http.Client
}

// WebhookPayload describes the JSON payload to be delivered to the configured webhook endpoint.
type WebhookPayload struct {
	Version string               `json:"_version"`
	MsgID   uuid.UUID            `json:"msg_id"`
	Payload types.MessagePayload `json:"payload"`
	Title   string               `json:"title"`
	Body    string               `json:"body"`
}

func NewWebhookHandler(cfg codersdk.NotificationsWebhookConfig, log slog.Logger) *WebhookHandler {
	return &WebhookHandler{cfg: cfg, log: log, cl: &http.Client{}}
}

func (w *WebhookHandler) Dispatcher(payload types.MessagePayload, titleTmpl, bodyTmpl string) (DeliveryFunc, error) {
	if w.cfg.Endpoint.String() == "" {
		return nil, xerrors.New("webhook endpoint not defined")
	}

	title, err := markdown.PlaintextFromMarkdown(titleTmpl)
	if err != nil {
		return nil, xerrors.Errorf("render title: %w", err)
	}
	body, err := markdown.PlaintextFromMarkdown(bodyTmpl)
	if err != nil {
		return nil, xerrors.Errorf("render body: %w", err)
	}

	return w.dispatch(payload, title, body, w.cfg.Endpoint.String()), nil
}

func (w *WebhookHandler) dispatch(msgPayload types.MessagePayload, title, body, endpoint string) DeliveryFunc {
	return func(ctx context.Context, msgID uuid.UUID) (retryable bool, err error) {
		// Prepare payload.
		payload := WebhookPayload{
			Version: "1.0",
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
			// Body could be quite long here, let's grab the first 500B and hope it contains useful debug info.
			respBody := make([]byte, 500)
			lr := io.LimitReader(resp.Body, int64(len(respBody)))
			n, err := lr.Read(respBody)
			if err != nil && !errors.Is(err, io.EOF) {
				return true, xerrors.Errorf("non-200 response (%d), read body: %w", resp.StatusCode, err)
			}
			w.log.Warn(ctx, "unsuccessful delivery", slog.F("status_code", resp.StatusCode),
				slog.F("response", respBody[:n]), slog.F("msg_id", msgID))
			return true, xerrors.Errorf("non-200 response (%d)", resp.StatusCode)
		}

		return false, nil
	}
}
