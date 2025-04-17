package notificationstest

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications"
)

type Enqueuer interface {
	notifications.Enqueuer

	Sent(matchers ...func(*FakeNotification) bool) []*FakeNotification
	Clear()
}

type FakeEnqueuer struct {
	mu    sync.Mutex
	sent  []*FakeNotification
	Store database.Store
}

type FakeNotification struct {
	UserID, TemplateID uuid.UUID
	Labels             map[string]string
	Data               map[string]any
	CreatedBy          string
	Targets            []uuid.UUID
}

func (f *FakeEnqueuer) Enqueue(ctx context.Context, userID, templateID uuid.UUID, labels map[string]string, createdBy string, targets ...uuid.UUID) ([]uuid.UUID, error) {
	return f.EnqueueWithData(ctx, userID, templateID, labels, nil, createdBy, targets...)
}

func (f *FakeEnqueuer) EnqueueWithData(ctx context.Context, userID, templateID uuid.UUID, labels map[string]string, data map[string]any, createdBy string, targets ...uuid.UUID) ([]uuid.UUID, error) {
	return f.enqueueWithDataLock(ctx, userID, templateID, labels, data, createdBy, targets...)
}

func (f *FakeEnqueuer) enqueueWithDataLock(ctx context.Context, userID, templateID uuid.UUID, labels map[string]string, data map[string]any, createdBy string, targets ...uuid.UUID) ([]uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := uuid.New()
	var err error

	// To avoid a duplicate notification, make a unique-enough payload out of what
	// we have.
	fakePayload := make(map[string]any)
	fakePayload["template_id"] = templateID
	fakePayload["labels"] = labels
	fakePayload["data"] = data
	fakePayloadBytes, err := json.Marshal(fakePayload)
	if err != nil {
		return nil, err
	}

	if err = f.Store.EnqueueNotificationMessage(ctx, database.EnqueueNotificationMessageParams{
		ID:                     id,
		UserID:                 userID,
		NotificationTemplateID: templateID,
		Payload:                fakePayloadBytes,
		Method:                 database.NotificationMethodInbox,
		Targets:                targets,
		CreatedBy:              createdBy,
		CreatedAt:              dbtime.Now(),
	}); err != nil {
		// TODO: just use the real thing. See https://github.com/coder/coder/issues/15481
		if strings.Contains(err.Error(), notifications.ErrCannotEnqueueDisabledNotification.Error()) {
			err = notifications.ErrCannotEnqueueDisabledNotification
		} else if database.IsUniqueViolation(err, database.UniqueNotificationMessagesDedupeHashIndex) {
			err = notifications.ErrDuplicate
		}
	}

	f.sent = append(f.sent, &FakeNotification{
		UserID:     userID,
		TemplateID: templateID,
		Labels:     labels,
		Data:       data,
		CreatedBy:  createdBy,
		Targets:    targets,
	})

	return []uuid.UUID{id}, err
}

func (f *FakeEnqueuer) Clear() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.sent = nil
}

func (f *FakeEnqueuer) Sent(matchers ...func(*FakeNotification) bool) []*FakeNotification {
	f.mu.Lock()
	defer f.mu.Unlock()

	sent := []*FakeNotification{}
	for _, notif := range f.sent {
		// Check this notification matches all given matchers
		matches := true
		for _, matcher := range matchers {
			if !matcher(notif) {
				matches = false
				break
			}
		}

		if matches {
			sent = append(sent, notif)
		}
	}

	return sent
}

func WithTemplateID(id uuid.UUID) func(*FakeNotification) bool {
	return func(n *FakeNotification) bool {
		return n.TemplateID == id
	}
}
