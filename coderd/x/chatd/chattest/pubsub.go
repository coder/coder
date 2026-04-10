package chattest

import (
	"context"
	"encoding/json"
	"sync"

	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
)

type delayedStatusKey struct {
	event  string
	status string
}

// DelayedStatusPubsub buffers selected status notifications until a test
// releases them. This lets tests exercise stale-notify races deterministically
// without depending on PostgreSQL delivery timing.
type DelayedStatusPubsub struct {
	inner dbpubsub.Pubsub

	mu              sync.Mutex
	delayEnabled    map[delayedStatusKey]bool
	delayedMessages map[delayedStatusKey][][]byte
	subscribed      map[string]bool
	subscribeWait   map[string]chan struct{}
	published       map[delayedStatusKey]bool
	publishWait     map[delayedStatusKey]chan struct{}
}

// NewDelayedStatusPubsub wraps a pubsub implementation with deterministic
// buffering for chosen status notifications.
func NewDelayedStatusPubsub(inner dbpubsub.Pubsub) *DelayedStatusPubsub {
	return &DelayedStatusPubsub{
		inner:           inner,
		delayEnabled:    make(map[delayedStatusKey]bool),
		delayedMessages: make(map[delayedStatusKey][][]byte),
		subscribed:      make(map[string]bool),
		subscribeWait:   make(map[string]chan struct{}),
		published:       make(map[delayedStatusKey]bool),
		publishWait:     make(map[delayedStatusKey]chan struct{}),
	}
}

// DelayStatus starts buffering matching status notifications instead of
// publishing them immediately.
func (p *DelayedStatusPubsub) DelayStatus(event, status string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.delayEnabled[delayedStatusKey{event: event, status: status}] = true
}

// ReleaseStatus flushes the buffered notifications for an event/status pair in
// publish order and resumes immediate delivery for later notifications.
func (p *DelayedStatusPubsub) ReleaseStatus(event, status string) error {
	key := delayedStatusKey{event: event, status: status}

	p.mu.Lock()
	delete(p.delayEnabled, key)
	messages := p.delayedMessages[key]
	delete(p.delayedMessages, key)
	p.mu.Unlock()

	for _, message := range messages {
		if err := p.inner.Publish(event, message); err != nil {
			return err
		}
	}
	return nil
}

// WaitForSubscribe blocks until a subscriber registers for the event.
func (p *DelayedStatusPubsub) WaitForSubscribe(ctx context.Context, event string) error {
	wait := p.subscribeWaiter(event)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-wait:
		return nil
	}
}

// WaitForStatusPublish blocks until a matching status notification is
// published, whether or not it is currently buffered.
func (p *DelayedStatusPubsub) WaitForStatusPublish(ctx context.Context, event, status string) error {
	wait := p.publishWaiter(delayedStatusKey{event: event, status: status})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-wait:
		return nil
	}
}

func (p *DelayedStatusPubsub) Subscribe(event string, listener dbpubsub.Listener) (func(), error) {
	cancel, err := p.inner.Subscribe(event, listener)
	if err == nil {
		p.markSubscribed(event)
	}
	return cancel, err
}

func (p *DelayedStatusPubsub) SubscribeWithErr(event string, listener dbpubsub.ListenerWithErr) (func(), error) {
	cancel, err := p.inner.SubscribeWithErr(event, listener)
	if err == nil {
		p.markSubscribed(event)
	}
	return cancel, err
}

func (p *DelayedStatusPubsub) Publish(event string, message []byte) error {
	status, ok := chatNotifyStatus(message)
	if !ok {
		return p.inner.Publish(event, message)
	}

	key := delayedStatusKey{event: event, status: status}
	p.markPublished(key)

	p.mu.Lock()
	delay := p.delayEnabled[key]
	if delay {
		p.delayedMessages[key] = append(p.delayedMessages[key], append([]byte(nil), message...))
	}
	p.mu.Unlock()
	if delay {
		return nil
	}

	return p.inner.Publish(event, message)
}

func (p *DelayedStatusPubsub) Close() error {
	return p.inner.Close()
}

func (p *DelayedStatusPubsub) markSubscribed(event string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.subscribed[event] {
		return
	}
	p.subscribed[event] = true
	if wait, ok := p.subscribeWait[event]; ok {
		close(wait)
		delete(p.subscribeWait, event)
	}
}

func (p *DelayedStatusPubsub) markPublished(key delayedStatusKey) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.published[key] {
		return
	}
	p.published[key] = true
	if wait, ok := p.publishWait[key]; ok {
		close(wait)
		delete(p.publishWait, key)
	}
}

func (p *DelayedStatusPubsub) subscribeWaiter(event string) chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.subscribeWaiterLocked(event)
}

func (p *DelayedStatusPubsub) subscribeWaiterLocked(event string) chan struct{} {
	if p.subscribed[event] {
		return closedSignal()
	}
	wait, ok := p.subscribeWait[event]
	if !ok {
		wait = make(chan struct{})
		p.subscribeWait[event] = wait
	}
	return wait
}

func (p *DelayedStatusPubsub) publishWaiter(key delayedStatusKey) chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.publishWaiterLocked(key)
}

func (p *DelayedStatusPubsub) publishWaiterLocked(key delayedStatusKey) chan struct{} {
	if p.published[key] {
		return closedSignal()
	}
	wait, ok := p.publishWait[key]
	if !ok {
		wait = make(chan struct{})
		p.publishWait[key] = wait
	}
	return wait
}

func chatNotifyStatus(message []byte) (string, bool) {
	var notify coderdpubsub.ChatStreamNotifyMessage
	if err := json.Unmarshal(message, &notify); err != nil {
		return "", false
	}
	if notify.Status == "" {
		return "", false
	}
	return notify.Status, true
}

func closedSignal() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}
