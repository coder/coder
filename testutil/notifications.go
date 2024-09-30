package testutil

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

type FakeNotificationsEnqueuer struct {
	mu   sync.Mutex
	Sent []*Notification
}

type Notification struct {
	UserID, TemplateID uuid.UUID
	Labels             map[string]string
	Data               map[string]any
	CreatedBy          string
	Targets            []uuid.UUID
}

func (f *FakeNotificationsEnqueuer) Enqueue(ctx context.Context, userID, templateID uuid.UUID, labels map[string]string, createdBy string, targets ...uuid.UUID) (*uuid.UUID, error) {
	return f.EnqueueWithData(ctx, userID, templateID, labels, nil, createdBy, targets...)
}

func (f *FakeNotificationsEnqueuer) EnqueueWithData(_ context.Context, userID, templateID uuid.UUID, labels map[string]string, data map[string]any, createdBy string, targets ...uuid.UUID) (*uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Sent = append(f.Sent, &Notification{
		UserID:     userID,
		TemplateID: templateID,
		Labels:     labels,
		Data:       data,
		CreatedBy:  createdBy,
		Targets:    targets,
	})

	id := uuid.New()
	return &id, nil
}

func (f *FakeNotificationsEnqueuer) Clear() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Sent = nil
}
