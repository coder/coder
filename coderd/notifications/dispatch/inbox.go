package dispatch
import (
	"fmt"
	"errors"
	"context"
	"encoding/json"
	"text/template"
	"cdr.dev/slog"
	"github.com/google/uuid"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications/types"
	markdown "github.com/coder/coder/v2/coderd/render"
)
type InboxStore interface {
	InsertInboxNotification(ctx context.Context, arg database.InsertInboxNotificationParams) (database.InboxNotification, error)
}
// InboxHandler is responsible for dispatching notification messages to the Coder Inbox.
type InboxHandler struct {
	log   slog.Logger
	store InboxStore
}
func NewInboxHandler(log slog.Logger, store InboxStore) *InboxHandler {
	return &InboxHandler{log: log, store: store}
}
func (s *InboxHandler) Dispatcher(payload types.MessagePayload, titleTmpl, bodyTmpl string, _ template.FuncMap) (DeliveryFunc, error) {
	subject, err := markdown.PlaintextFromMarkdown(titleTmpl)
	if err != nil {
		return nil, fmt.Errorf("render subject: %w", err)
	}
	htmlBody, err := markdown.PlaintextFromMarkdown(bodyTmpl)
	if err != nil {
		return nil, fmt.Errorf("render html body: %w", err)
	}
	return s.dispatch(payload, subject, htmlBody), nil
}
func (s *InboxHandler) dispatch(payload types.MessagePayload, title, body string) DeliveryFunc {
	return func(ctx context.Context, msgID uuid.UUID) (bool, error) {
		userID, err := uuid.Parse(payload.UserID)
		if err != nil {
			return false, fmt.Errorf("parse user ID: %w", err)
		}
		templateID, err := uuid.Parse(payload.NotificationTemplateID)
		if err != nil {
			return false, fmt.Errorf("parse template ID: %w", err)
		}
		actions, err := json.Marshal(payload.Actions)
		if err != nil {
			return false, fmt.Errorf("marshal actions: %w", err)
		}
		// nolint:exhaustruct
		_, err = s.store.InsertInboxNotification(ctx, database.InsertInboxNotificationParams{
			ID:         msgID,
			UserID:     userID,
			TemplateID: templateID,
			Targets:    payload.Targets,
			Title:      title,
			Content:    body,
			Actions:    actions,
			CreatedAt:  dbtime.Now(),
		})
		if err != nil {
			return false, fmt.Errorf("insert inbox notification: %w", err)
		}
		return false, nil
	}
}
