package notifications

import (
	"context"
	"text/template"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/notifications/dispatch"
	"github.com/coder/coder/v2/coderd/notifications/types"
)

// Store defines the API between the notifications system and the storage.
// This abstraction is in place so that we can intercept the direct database interactions, or (later) swap out these calls
// with dRPC calls should we want to split the notifiers out into their own component for high availability/throughput.
// TODO: don't use database types here
type Store interface {
	AcquireNotificationMessages(ctx context.Context, params database.AcquireNotificationMessagesParams) ([]database.AcquireNotificationMessagesRow, error)
	BulkMarkNotificationMessagesSent(ctx context.Context, arg database.BulkMarkNotificationMessagesSentParams) (int64, error)
	BulkMarkNotificationMessagesFailed(ctx context.Context, arg database.BulkMarkNotificationMessagesFailedParams) (int64, error)
	EnqueueNotificationMessage(ctx context.Context, arg database.EnqueueNotificationMessageParams) error
	FetchNewMessageMetadata(ctx context.Context, arg database.FetchNewMessageMetadataParams) (database.FetchNewMessageMetadataRow, error)
	GetNotificationMessagesByStatus(ctx context.Context, arg database.GetNotificationMessagesByStatusParams) ([]database.NotificationMessage, error)
	GetNotificationsSettings(ctx context.Context) (string, error)
	GetApplicationName(ctx context.Context) (string, error)
	GetLogoURL(ctx context.Context) (string, error)

	InsertInboxNotification(ctx context.Context, arg database.InsertInboxNotificationParams) (database.InboxNotification, error)
}

// Handler is responsible for preparing and delivering a notification by a given method.
type Handler interface {
	// Dispatcher constructs a DeliveryFunc to be used for delivering a notification via the chosen method.
	Dispatcher(payload types.MessagePayload, title, body string, helpers template.FuncMap) (dispatch.DeliveryFunc, error)
}

// Enqueuer enqueues a new notification message in the store and returns ids of the messages inserted - one for each target.
type Enqueuer interface {
	Enqueue(ctx context.Context, userID, templateID uuid.UUID, labels map[string]string, createdBy string, targets ...uuid.UUID) ([]*uuid.UUID, error)
	EnqueueWithData(ctx context.Context, userID, templateID uuid.UUID, labels map[string]string, data map[string]any, createdBy string, targets ...uuid.UUID) ([]*uuid.UUID, error)
}
