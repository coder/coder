package testutil

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

type FakeNotificationEnqueuer struct {
	mu   sync.Mutex
	Sent []*Notification
}

type Notification struct {
	UserID, TemplateID uuid.UUID
	Labels             map[string]string
	CreatedBy          string
	Targets            []uuid.UUID
}

func (f *FakeNotificationEnqueuer) Enqueue(_ context.Context, userID, templateID uuid.UUID, labels map[string]string, createdBy string, targets ...uuid.UUID) (*uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Sent = append(f.Sent, &Notification{
		UserID:     userID,
		TemplateID: templateID,
		Labels:     labels,
		CreatedBy:  createdBy,
		Targets:    targets,
	})

	id := uuid.New()
	return &id, nil
}

func (f *FakeNotificationEnqueuer) Clear() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Sent = nil
}
