package notifications

import (
	"context"
	"encoding/json"
	"text/template"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/notifications/render"
	"github.com/coder/coder/v2/coderd/notifications/types"
	"github.com/coder/coder/v2/codersdk"
)

type StoreEnqueuer struct {
	store Store
	log   slog.Logger

	// TODO: expand this to allow for each notification to have custom delivery methods, or multiple, or none.
	// 		 For example, Larry might want email notifications for "workspace deleted" notifications, but Harry wants
	//		 Slack notifications, and Mary doesn't want any.
	method database.NotificationMethod
	// helpers holds a map of template funcs which are used when rendering templates. These need to be passed in because
	// the template funcs will return values which are inappropriately encapsulated in this struct.
	helpers template.FuncMap
}

// NewStoreEnqueuer creates an Enqueuer implementation which can persist notification messages in the store.
func NewStoreEnqueuer(cfg codersdk.NotificationsConfig, store Store, helpers template.FuncMap, log slog.Logger) (*StoreEnqueuer, error) {
	var method database.NotificationMethod
	if err := method.Scan(cfg.Method.String()); err != nil {
		return nil, xerrors.Errorf("given notification method %q is invalid", cfg.Method)
	}

	return &StoreEnqueuer{
		store:   store,
		log:     log,
		method:  method,
		helpers: helpers,
	}, nil
}

// Enqueue queues a notification message for later delivery.
// Messages will be dequeued by a notifier later and dispatched.
func (s *StoreEnqueuer) Enqueue(ctx context.Context, userID, templateID uuid.UUID, labels map[string]string, createdBy string, targets ...uuid.UUID) (*uuid.UUID, error) {
	payload, err := s.buildPayload(ctx, userID, templateID, labels)
	if err != nil {
		s.log.Warn(ctx, "failed to build payload", slog.F("template_id", templateID), slog.F("user_id", userID), slog.Error(err))
		return nil, xerrors.Errorf("enqueue notification (payload build): %w", err)
	}

	input, err := json.Marshal(payload)
	if err != nil {
		return nil, xerrors.Errorf("failed encoding input labels: %w", err)
	}

	id := uuid.New()
	err = s.store.EnqueueNotificationMessage(ctx, database.EnqueueNotificationMessageParams{
		ID:                     id,
		UserID:                 userID,
		NotificationTemplateID: templateID,
		Method:                 s.method,
		Payload:                input,
		Targets:                targets,
		CreatedBy:              createdBy,
	})
	if err != nil {
		s.log.Warn(ctx, "failed to enqueue notification", slog.F("template_id", templateID), slog.F("input", input), slog.Error(err))
		return nil, xerrors.Errorf("enqueue notification: %w", err)
	}

	s.log.Debug(ctx, "enqueued notification", slog.F("msg_id", id))
	return &id, nil
}

// buildPayload creates the payload that the notification will for variable substitution and/or routing.
// The payload contains information about the recipient, the event that triggered the notification, and any subsequent
// actions which can be taken by the recipient.
func (s *StoreEnqueuer) buildPayload(ctx context.Context, userID, templateID uuid.UUID, labels map[string]string) (*types.MessagePayload, error) {
	metadata, err := s.store.FetchNewMessageMetadata(ctx, database.FetchNewMessageMetadataParams{
		UserID:                 userID,
		NotificationTemplateID: templateID,
	})
	if err != nil {
		return nil, xerrors.Errorf("new message metadata: %w", err)
	}

	payload := types.MessagePayload{
		Version: "1.0",

		NotificationName: metadata.NotificationName,

		UserID:    metadata.UserID.String(),
		UserEmail: metadata.UserEmail,
		UserName:  metadata.UserName,

		Labels: labels,
		// No actions yet
	}

	// Execute any templates in actions.
	out, err := render.GoTemplate(string(metadata.Actions), payload, s.helpers)
	if err != nil {
		return nil, xerrors.Errorf("render actions: %w", err)
	}
	metadata.Actions = []byte(out)

	var actions []types.TemplateAction
	if err = json.Unmarshal(metadata.Actions, &actions); err != nil {
		return nil, xerrors.Errorf("new message metadata: parse template actions: %w", err)
	}
	payload.Actions = actions
	return &payload, nil
}

// NoopEnqueuer implements the Enqueuer interface but performs a noop.
type NoopEnqueuer struct{}

// NewNoopEnqueuer builds a NoopEnqueuer which is used to fulfill the contract for enqueuing notifications, if ExperimentNotifications is not set.
func NewNoopEnqueuer() *NoopEnqueuer {
	return &NoopEnqueuer{}
}

func (*NoopEnqueuer) Enqueue(context.Context, uuid.UUID, uuid.UUID, map[string]string, string, ...uuid.UUID) (*uuid.UUID, error) {
	// nolint:nilnil // irrelevant.
	return nil, nil
}
