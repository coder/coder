// Package messagepartbuffer stores the transient message-part stream that a
// chat worker emits before those parts are committed to durable chat history.
//
// Chat generation has two consumers with different timing. Stream endpoints
// need to forward parts immediately, while interruption handling may need to
// recover the partial assistant or tool message and commit it. Buffer groups
// parts by an episode key that includes the chat, history version, and
// generation attempt so stale workers and late subscribers do not mix parts
// from different generations.
//
// Episodes are intentionally in-memory. They are closed when a generation
// attempt ends, then retained briefly so stream subscribers and interruption
// cleanup can drain the final parts. The cleanup loop removes closed episodes
// after the retention window. Never-created placeholders are removed during
// subscriber teardown, when the last early subscriber leaves.
package messagepartbuffer

import (
	"container/heap"
	"context"
	"encoding/json"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const (
	defaultMaxEpisodeBytes        = int64(1024 * 1024)
	defaultClosedEpisodeRetention = 15 * time.Second
	defaultSubscriberSendTimeout  = 10 * time.Second
)

var (
	// ErrEpisodeExists means the episode already exists.
	ErrEpisodeExists = xerrors.New("message part episode already exists")
	// ErrEpisodeNotFound means the episode has not been created.
	ErrEpisodeNotFound = xerrors.New("message part episode not found")
	// ErrEpisodeClosed means the episode no longer accepts parts.
	ErrEpisodeClosed = xerrors.New("message part episode closed")
	// ErrEpisodeFull means the episode byte limit would be exceeded.
	ErrEpisodeFull = xerrors.New("message part episode full")
	// ErrMessagePartBufferClosed means the whole buffer is closed.
	ErrMessagePartBufferClosed = xerrors.New("message part buffer closed")
)

// Key identifies a buffered message part episode.
type Key struct {
	ChatID            uuid.UUID
	HistoryVersion    int64
	GenerationAttempt int64
}

// Part is a buffered chat message part with its sequence number.
type Part struct {
	Seq         int64
	Role        codersdk.ChatMessageRole
	MessagePart codersdk.ChatMessagePart
}

type partJSON struct {
	Seq  int64                    `json:"seq"`
	Role codersdk.ChatMessageRole `json:"role"`
	Part codersdk.ChatMessagePart `json:"part"`
}

func (p Part) jsonValue() partJSON {
	return partJSON{
		Seq:  p.Seq,
		Role: p.Role,
		Part: p.MessagePart,
	}
}

// Options configures a Buffer.
type Options struct {
	MaxEpisodeBytes        int64
	ClosedEpisodeRetention time.Duration
	SubscriberSendTimeout  time.Duration
	Clock                  quartz.Clock
}

// Buffer stores streamed message parts by episode.
type Buffer struct {
	mu             sync.Mutex
	opts           Options
	episodes       map[Key]*episodeState
	closedEpisodes closedEpisodeHeap
	closed         bool
	done           chan struct{}
}

type episodeState struct {
	created        bool
	closed         bool
	closedAt       time.Time
	closedHeapItem *closedEpisodeItem
	parts          []Part
	bytes          int64
	subscribers    map[*episodeSubscriber]struct{}
}

type closedEpisodeItem struct {
	key      Key
	closedAt time.Time
}

type closedEpisodeHeap []*closedEpisodeItem

func (h closedEpisodeHeap) Len() int {
	return len(h)
}

func (h closedEpisodeHeap) Less(i, j int) bool {
	return h[i].closedAt.Before(h[j].closedAt)
}

func (h closedEpisodeHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *closedEpisodeHeap) Push(value any) {
	item, ok := value.(*closedEpisodeItem)
	if !ok {
		// The reason we panic here instead of returning an error is that
		// closedEpisodeHeap implements the https://pkg.go.dev/container/heap interface.
		// We must accept an any type and we must not return an error.
		panic("closed episode heap received invalid item")
	}
	*h = append(*h, item)
}

func (h *closedEpisodeHeap) Pop() any {
	old := *h
	last := old[len(old)-1]
	old[len(old)-1] = nil
	*h = old[:len(old)-1]
	return last
}

type episodeSubscriber struct {
	out      chan Part
	notifyCh chan struct{}
	stopCh   chan struct{}
	next     int
	stopOnce sync.Once
}

// New returns a message part buffer.
func New(options Options) *Buffer {
	if options.MaxEpisodeBytes <= 0 {
		options.MaxEpisodeBytes = defaultMaxEpisodeBytes
	}
	if options.ClosedEpisodeRetention <= 0 {
		options.ClosedEpisodeRetention = defaultClosedEpisodeRetention
	}
	if options.SubscriberSendTimeout <= 0 {
		options.SubscriberSendTimeout = defaultSubscriberSendTimeout
	}
	if options.Clock == nil {
		options.Clock = quartz.NewReal()
	}
	buffer := &Buffer{
		opts:     options,
		episodes: make(map[Key]*episodeState),
		// done is unbuffered because it's only ever closed - never sent on.
		done: make(chan struct{}),
	}
	buffer.startCleanupLoop()
	return buffer
}

// CreateEpisode creates a new episode.
//
// Subscribers may attach before an episode is created. Creating the episode
// makes it eligible to receive parts; the first AddPart wakes early subscribers.
func (b *Buffer) CreateEpisode(key Key) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return ErrMessagePartBufferClosed
	}
	b.gcClosedEpisodesLocked(b.opts.Clock.Now("message-part-buffer", "create"))
	episode := b.getOrCreateEpisodeLocked(key)
	if episode.created {
		return ErrEpisodeExists
	}
	episode.markCreated()
	return nil
}

// AddPart appends a part to an existing episode.
//
// Parts receive contiguous sequence numbers so stream endpoints can detect
// stale or broken episode subscriptions before forwarding data to clients.
func (b *Buffer) AddPart(key Key, role codersdk.ChatMessageRole, part codersdk.ChatMessagePart) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return ErrMessagePartBufferClosed
	}
	episode, err := b.getEpisodeLocked(key)
	if err != nil {
		return err
	}
	if episode.closed {
		return ErrEpisodeClosed
	}
	buffered := Part{
		Seq:         int64(len(episode.parts) + 1),
		Role:        role,
		MessagePart: part,
	}
	sizeBytes, err := serializedPartBytes(buffered)
	if err != nil {
		return err
	}
	if episode.bytes+sizeBytes > b.opts.MaxEpisodeBytes {
		return ErrEpisodeFull
	}
	episode.parts = append(episode.parts, buffered)
	episode.bytes += sizeBytes
	episode.notifySubscribers()
	return nil
}

// CloseEpisode marks an episode closed.
//
// Closing creates the episode if it did not exist yet. This lets interruption
// cleanup converge when a worker exits before it publishes any parts.
func (b *Buffer) CloseEpisode(key Key) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return ErrMessagePartBufferClosed
	}
	episode := b.getOrCreateEpisodeLocked(key)
	if !episode.close(b.opts.Clock.Now("message-part-buffer", "close")) {
		return nil
	}
	b.queueClosedEpisodeLocked(key, episode)
	episode.notifySubscribers()
	return nil
}

// GetParts returns a snapshot of buffered parts for an episode.
//
// The returned slice is detached from the buffer so callers can process it
// without holding the buffer lock.
func (b *Buffer) GetParts(key Key) ([]Part, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, ErrMessagePartBufferClosed
	}
	b.gcClosedEpisodesLocked(b.opts.Clock.Now("message-part-buffer", "get"))
	episode, err := b.getEpisodeLocked(key)
	if err != nil {
		return nil, err
	}
	return slices.Clone(episode.parts), nil
}

// SubscribeToEpisode replays existing parts and streams new parts.
//
// Subscribers may attach before CreateEpisode is called. In that case the
// subscription stays idle until the first part added, closure, cancellation,
// or buffer shutdown. The returned cancel function is idempotent.
func (b *Buffer) SubscribeToEpisode(ctx context.Context, key Key) (<-chan Part, func(), error) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, nil, ErrMessagePartBufferClosed
	}
	episode := b.getOrCreateEpisodeLocked(key)
	subscriber := &episodeSubscriber{
		// out is unbuffered so the delivery goroutine only advances once the
		// subscriber has accepted each part. The send timeout bounds how long
		// an unresponsive subscriber can keep its episode retained.
		out: make(chan Part),
		// notifyCh is a one-slot wakeup channel. Additional wakeups can be
		// coalesced because the delivery goroutine copies all available parts
		// each time it wakes.
		notifyCh: make(chan struct{}, 1),
		// stopCh is unbuffered because stop only closes it. Closing does not
		// block and every select that observes it treats it as cancellation.
		stopCh: make(chan struct{}),
	}
	if episode.subscribers == nil {
		episode.subscribers = make(map[*episodeSubscriber]struct{})
	}
	episode.subscribers[subscriber] = struct{}{}
	notifySubscriber(subscriber)
	b.mu.Unlock()

	go b.deliverSubscriber(ctx, key, subscriber)
	cancel := func() {
		b.cancelSubscriber(key, subscriber)
	}
	return subscriber.out, cancel, nil
}

// Close closes the buffer and all pending subscriptions.
func (b *Buffer) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	close(b.done)
	for _, episode := range b.episodes {
		for subscriber := range episode.subscribers {
			b.stopSubscriberLocked(episode, subscriber)
		}
	}
	b.mu.Unlock()
}

func (b *Buffer) startCleanupLoop() {
	ticker := b.opts.Clock.NewTicker(b.opts.ClosedEpisodeRetention, "message-part-buffer", "cleanup")
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				b.mu.Lock()
				if b.closed {
					b.mu.Unlock()
					return
				}
				b.gcClosedEpisodesLocked(b.opts.Clock.Now("message-part-buffer", "cleanup"))
				b.mu.Unlock()
			case <-b.done:
				return
			}
		}
	}()
}

func (b *Buffer) gcClosedEpisodesLocked(now time.Time) {
	cutoff := now.Add(-b.opts.ClosedEpisodeRetention)
	type retainedEpisode struct {
		key     Key
		episode *episodeState
	}
	retained := make([]retainedEpisode, 0)
	for b.closedEpisodes.Len() > 0 {
		item := b.closedEpisodes[0]
		if item.closedAt.After(cutoff) {
			break
		}
		popped, ok := heap.Pop(&b.closedEpisodes).(*closedEpisodeItem)
		if !ok || popped != item {
			continue
		}
		episode := b.episodes[item.key]
		if episode == nil || episode.closedHeapItem != item || !episode.closed {
			continue
		}
		episode.closedHeapItem = nil
		if len(episode.subscribers) > 0 {
			retained = append(retained, retainedEpisode{key: item.key, episode: episode})
			continue
		}
		delete(b.episodes, item.key)
	}
	for _, item := range retained {
		if b.episodes[item.key] != item.episode || !item.episode.closed || item.episode.closedHeapItem != nil {
			continue
		}
		b.queueClosedEpisodeLocked(item.key, item.episode)
	}
}

func (b *Buffer) queueClosedEpisodeLocked(key Key, episode *episodeState) {
	if episode.closedHeapItem != nil {
		return
	}
	item := &closedEpisodeItem{key: key, closedAt: episode.closedAt}
	episode.closedHeapItem = item
	heap.Push(&b.closedEpisodes, item)
}

func (b *Buffer) getOrCreateEpisodeLocked(key Key) *episodeState {
	episode := b.episodes[key]
	if episode != nil {
		return episode
	}
	episode = &episodeState{}
	b.episodes[key] = episode
	return episode
}

func (b *Buffer) getEpisodeLocked(key Key) (*episodeState, error) {
	episode := b.episodes[key]
	if episode == nil || !episode.created {
		return nil, ErrEpisodeNotFound
	}
	return episode, nil
}

func (b *Buffer) subscriberParts(key Key, subscriber *episodeSubscriber) (parts []Part, closed bool, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, false, false
	}
	episode := b.episodes[key]
	if episode == nil {
		return nil, false, false
	}
	if !episode.created {
		return nil, false, true
	}
	if subscriber.next > len(episode.parts) {
		return nil, false, false
	}
	parts = slices.Clone(episode.parts[subscriber.next:])
	subscriber.next = len(episode.parts)
	return parts, episode.closed && subscriber.next == len(episode.parts), true
}

func (b *Buffer) deliverSubscriber(ctx context.Context, key Key, subscriber *episodeSubscriber) {
	defer close(subscriber.out)
	defer b.removeSubscriber(key, subscriber)
	for {
		parts, closed, ok := b.subscriberParts(key, subscriber)
		if !ok {
			return
		}
		for _, part := range parts {
			if !b.sendSubscriberPart(ctx, subscriber, part) {
				return
			}
		}
		if closed {
			return
		}
		select {
		case <-subscriber.notifyCh:
		case <-subscriber.stopCh:
			return
		case <-ctx.Done():
			return
		case <-b.done:
			return
		}
	}
}

func (b *Buffer) sendSubscriberPart(ctx context.Context, subscriber *episodeSubscriber, part Part) bool {
	timer := b.opts.Clock.NewTimer(b.opts.SubscriberSendTimeout, "message-part-buffer", "subscriber-send")
	defer timer.Stop()
	select {
	case subscriber.out <- part:
		return true
	case <-timer.C:
		return false
	case <-subscriber.stopCh:
		return false
	case <-ctx.Done():
		return false
	case <-b.done:
		return false
	}
}

func (b *Buffer) cancelSubscriber(key Key, subscriber *episodeSubscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()
	episode := b.episodes[key]
	if episode != nil {
		b.stopSubscriberLocked(episode, subscriber)
		return
	}
	subscriber.stop()
}

func (b *Buffer) removeSubscriber(key Key, subscriber *episodeSubscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()
	episode := b.episodes[key]
	if episode == nil {
		return
	}
	delete(episode.subscribers, subscriber)
	if len(episode.subscribers) != 0 {
		return
	}
	switch {
	case episode.closed:
		b.queueClosedEpisodeLocked(key, episode)
	case !episode.created:
		// SubscribeToEpisode inserts a placeholder state for unknown keys so
		// that CreateEpisode can adopt subscribers that arrive early. Once the
		// last subscriber leaves a still-uncreated episode, no CreateEpisode or
		// CloseEpisode call will ever reclaim it, so delete it here to avoid
		// leaking the map entry for the lifetime of the buffer.
		delete(b.episodes, key)
	}
}

func (*Buffer) stopSubscriberLocked(episode *episodeState, subscriber *episodeSubscriber) {
	delete(episode.subscribers, subscriber)
	subscriber.stop()
}

func notifySubscriber(subscriber *episodeSubscriber) {
	select {
	case subscriber.notifyCh <- struct{}{}:
	default:
	}
}

func (e *episodeState) markCreated() {
	e.created = true
}

// close marks the episode closed and returns false if it was already closed.
func (e *episodeState) close(now time.Time) bool {
	e.markCreated()
	if e.closed {
		return false
	}
	e.closed = true
	e.closedAt = now
	return true
}

func (e *episodeState) notifySubscribers() {
	for subscriber := range e.subscribers {
		notifySubscriber(subscriber)
	}
}

func (s *episodeSubscriber) stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
}

func serializedPartBytes(part Part) (int64, error) {
	data, err := json.Marshal(part.jsonValue())
	if err != nil {
		return 0, err
	}
	return int64(len(data)), nil
}
