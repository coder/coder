package notificationstest

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

type FakeEnqueuer struct {
	authorizer rbac.Authorizer
	mu         sync.Mutex
	sent       []*FakeNotification
}

var _ notifications.Enqueuer = &FakeEnqueuer{}

func NewFakeEnqueuer() notifications.Enqueuer {
	return &FakeEnqueuer{}
}

type FakeNotification struct {
	UserID, TemplateID uuid.UUID
	Labels             map[string]string
	Data               map[string]any
	CreatedBy          string
	Targets            []uuid.UUID
}

// TODO: replace this with actual calls to dbauthz.
// See: https://github.com/coder/coder/issues/15481
func (f *FakeEnqueuer) assertRBACNoLock(ctx context.Context) {
	if f.mu.TryLock() {
		panic("Developer error: do not call assertRBACNoLock outside of a mutex lock!")
	}

	// If we get here, we are locked.
	if f.authorizer == nil {
		f.authorizer = rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())
	}

	act, ok := dbauthz.ActorFromContext(ctx)
	if !ok {
		panic("Developer error: no actor in context, you may need to use dbauthz.AsNotifier(ctx)")
	}

	for _, a := range []policy.Action{policy.ActionCreate, policy.ActionRead} {
		err := f.authorizer.Authorize(ctx, act, a, rbac.ResourceNotificationMessage)
		if err == nil {
			return
		}

		if rbac.IsUnauthorizedError(err) {
			panic(fmt.Sprintf("Developer error: not authorized to %s %s. "+
				"Ensure that you are using dbauthz.AsXXX with an actor that has "+
				"policy.ActionCreate on rbac.ResourceNotificationMessage", a, rbac.ResourceNotificationMessage.Type))
		}
		panic("Developer error: failed to check auth:" + err.Error())
	}
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
	f.assertRBACNoLock(ctx)

	f.sent = append(f.sent, &FakeNotification{
		UserID:     userID,
		TemplateID: templateID,
		Labels:     labels,
		Data:       data,
		CreatedBy:  createdBy,
		Targets:    targets,
	})

	id := uuid.New()
	return []uuid.UUID{id}, nil
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
