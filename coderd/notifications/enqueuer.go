package notifications

import (
	"context"
	"encoding/json"
	"strings"
	"text/template"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/quartz"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications/render"
	"github.com/coder/coder/v2/coderd/notifications/types"
	"github.com/coder/coder/v2/codersdk"
)

var (
	ErrCannotEnqueueDisabledNotification = xerrors.New("notification is not enabled")
	ErrDuplicate                         = xerrors.New("duplicate notification")
)

type StoreEnqueuer struct {
	store Store
	log   slog.Logger

	defaultMethod database.NotificationMethod
	// helpers holds a map of template funcs which are used when rendering templates. These need to be passed in because
	// the template funcs will return values which are inappropriately encapsulated in this struct.
	helpers template.FuncMap
	// Used to manipulate time in tests.
	clock quartz.Clock
}

// NewStoreEnqueuer creates an Enqueuer implementation which can persist notification messages in the store.
func NewStoreEnqueuer(cfg codersdk.NotificationsConfig, store Store, helpers template.FuncMap, log slog.Logger, clock quartz.Clock) (*StoreEnqueuer, error) {
	var method database.NotificationMethod
	if err := method.Scan(cfg.Method.String()); err != nil {
		return nil, xerrors.Errorf("given notification method %q is invalid", cfg.Method)
	}

	return &StoreEnqueuer{
		store:         store,
		log:           log,
		defaultMethod: method,
		helpers:       helpers,
		clock:         clock,
	}, nil
}

// Enqueue queues a notification message for later delivery, assumes no structured input data.
func (s *StoreEnqueuer) Enqueue(ctx context.Context, userID, templateID uuid.UUID, labels map[string]string, createdBy string, targets ...uuid.UUID) ([]uuid.UUID, error) {
	return s.EnqueueWithData(ctx, userID, templateID, labels, nil, createdBy, targets...)
}

// Enqueue queues a notification message for later delivery.
// Messages will be dequeued by a notifier later and dispatched.
func (s *StoreEnqueuer) EnqueueWithData(ctx context.Context, userID, templateID uuid.UUID, labels map[string]string, data map[string]any, createdBy string, targets ...uuid.UUID) ([]uuid.UUID, error) {
	metadata, err := s.store.FetchNewMessageMetadata(ctx, database.FetchNewMessageMetadataParams{
		UserID:                 userID,
		NotificationTemplateID: templateID,
	})
	if err != nil {
		s.log.Warn(ctx, "failed to fetch message metadata", slog.F("template_id", templateID), slog.F("user_id", userID), slog.Error(err))
		return nil, xerrors.Errorf("new message metadata: %w", err)
	}

	dispatchMethod := s.defaultMethod
	if metadata.CustomMethod.Valid {
		dispatchMethod = metadata.CustomMethod.NotificationMethod
	}

	payload, err := s.buildPayload(metadata, labels, data)
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
		Method:                 dispatchMethod,
		Payload:                input,
		Targets:                targets,
		CreatedBy:              createdBy,
		CreatedAt:              dbtime.Time(s.clock.Now().UTC()),
	})
	if err != nil {
		// We have a trigger on the notification_messages table named `inhibit_enqueue_if_disabled` which prevents messages
		// from being enqueued if the user has disabled them via notification_preferences. The trigger will fail the insertion
		// with the message "cannot enqueue message: user has disabled this notification".
		//
		// This is more efficient than fetching the user's preferences for each enqueue, and centralizes the business logic.
		if strings.Contains(err.Error(), ErrCannotEnqueueDisabledNotification.Error()) {
			return nil, ErrCannotEnqueueDisabledNotification
		}

		// If the enqueue fails due to a dedupe hash conflict, this means that a notification has already been enqueued
		// today with identical properties. It's far simpler to prevent duplicate sends in this central manner, rather than
		// having each notification enqueue handle its own logic.
		if database.IsUniqueViolation(err, database.UniqueNotificationMessagesDedupeHashIndex) {
			return nil, ErrDuplicate
		}

		s.log.Warn(ctx, "failed to enqueue notification", slog.F("template_id", templateID), slog.F("input", input), slog.Error(err))
		return nil, xerrors.Errorf("enqueue notification: %w", err)
	}

	s.log.Debug(ctx, "enqueued notification", slog.F("msg_id", id))
	return []uuid.UUID{id}, nil
}

// buildPayload creates the payload that the notification will for variable substitution and/or routing.
// The payload contains information about the recipient, the event that triggered the notification, and any subsequent
// actions which can be taken by the recipient.
func (s *StoreEnqueuer) buildPayload(metadata database.FetchNewMessageMetadataRow, labels map[string]string, data map[string]any) (*types.MessagePayload, error) {
	payload := types.MessagePayload{
		Version: "1.1",

		NotificationName:       metadata.NotificationName,
		NotificationTemplateID: metadata.NotificationTemplateID.String(),

		UserID:       metadata.UserID.String(),
		UserEmail:    metadata.UserEmail,
		UserName:     metadata.UserName,
		UserUsername: metadata.UserUsername,

		Labels: labels,
		Data:   data,

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

func (*NoopEnqueuer) Enqueue(context.Context, uuid.UUID, uuid.UUID, map[string]string, string, ...uuid.UUID) ([]uuid.UUID, error) {
	// nolint:nilnil // irrelevant.
	return nil, nil
}

func (*NoopEnqueuer) EnqueueWithData(context.Context, uuid.UUID, uuid.UUID, map[string]string, map[string]any, string, ...uuid.UUID) ([]uuid.UUID, error) {
	// nolint:nilnil // irrelevant.
	return nil, nil
}
