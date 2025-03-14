package dispatch
import (
	"fmt"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"text/template"
	"github.com/google/uuid"
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
	Version       string               `json:"_version"`
	MsgID         uuid.UUID            `json:"msg_id"`
	Payload       types.MessagePayload `json:"payload"`
	Title         string               `json:"title"`
	TitleMarkdown string               `json:"title_markdown"`
	Body          string               `json:"body"`
	BodyMarkdown  string               `json:"body_markdown"`
}
func NewWebhookHandler(cfg codersdk.NotificationsWebhookConfig, log slog.Logger) *WebhookHandler {
	return &WebhookHandler{cfg: cfg, log: log, cl: &http.Client{}}
}
func (w *WebhookHandler) Dispatcher(payload types.MessagePayload, titleMarkdown, bodyMarkdown string, _ template.FuncMap) (DeliveryFunc, error) {
	if w.cfg.Endpoint.String() == "" {
		return nil, errors.New("webhook endpoint not defined")
	}
	titlePlaintext, err := markdown.PlaintextFromMarkdown(titleMarkdown)
	if err != nil {
		return nil, fmt.Errorf("render title: %w", err)
	}
	bodyPlaintext, err := markdown.PlaintextFromMarkdown(bodyMarkdown)
	if err != nil {
		return nil, fmt.Errorf("render body: %w", err)
	}
	return w.dispatch(payload, titlePlaintext, titleMarkdown, bodyPlaintext, bodyMarkdown, w.cfg.Endpoint.String()), nil
}
func (w *WebhookHandler) dispatch(msgPayload types.MessagePayload, titlePlaintext, titleMarkdown, bodyPlaintext, bodyMarkdown, endpoint string) DeliveryFunc {
	return func(ctx context.Context, msgID uuid.UUID) (retryable bool, err error) {
		// Prepare payload.
		payload := WebhookPayload{
			Version:       "1.1",
			MsgID:         msgID,
			Title:         titlePlaintext,
			TitleMarkdown: titleMarkdown,
			Body:          bodyPlaintext,
			BodyMarkdown:  bodyMarkdown,
			Payload:       msgPayload,
		}
		m, err := json.Marshal(payload)
		if err != nil {
			return false, fmt.Errorf("marshal payload: %v", err)
		}
		// Prepare request.
		// Outer context has a deadline (see CODER_NOTIFICATIONS_DISPATCH_TIMEOUT).
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(m))
		if err != nil {
			return false, fmt.Errorf("create HTTP request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Message-Id", msgID.String())
		// Send request.
		resp, err := w.cl.Do(req)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return true, fmt.Errorf("request timeout: %w", err)
			}
			return true, fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()
		// Handle response.
		if resp.StatusCode/100 > 2 {
			// Body could be quite long here, let's grab the first 512B and hope it contains useful debug info.
			respBody := make([]byte, 512)
			lr := io.LimitReader(resp.Body, int64(len(respBody)))
			n, err := lr.Read(respBody)
			if err != nil && !errors.Is(err, io.EOF) {
				return true, fmt.Errorf("non-2xx response (%d), read body: %w", resp.StatusCode, err)
			}
			w.log.Warn(ctx, "unsuccessful delivery", slog.F("status_code", resp.StatusCode),
				slog.F("response", string(respBody[:n])), slog.F("msg_id", msgID))
			return true, fmt.Errorf("non-2xx response (%d)", resp.StatusCode)
		}
		return false, nil
	}
}
