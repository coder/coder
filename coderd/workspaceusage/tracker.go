package workspaceusage

import (
	"context"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
)

var DefaultFlushInterval = 60 * time.Second

// Store is a subset of database.Store
type Store interface {
	BatchUpdateWorkspaceLastUsedAt(context.Context, database.BatchUpdateWorkspaceLastUsedAtParams) error
}

// Tracker tracks and de-bounces updates to workspace usage activity.
// It keeps an internal map of workspace IDs that have been used and
// periodically flushes this to its configured Store.
type Tracker struct {
	log      slog.Logger            // you know, for logs
	mut      sync.Mutex             // protects m
	m        map[uuid.UUID]struct{} // stores workspace ids
	s        Store                  // for flushing data
	tickCh   <-chan time.Time       // controls flush interval
	stopTick func()                 // stops flushing
	stopCh   chan struct{}          // signals us to stop
	stopOnce sync.Once              // because you only stop once
	doneCh   chan struct{}          // signifies that we have stopped
	flushCh  chan int               // used for testing.
}

// New returns a new Tracker. It is the caller's responsibility
// to call Close().
func New(s Store, opts ...Option) *Tracker {
	hb := &Tracker{
		log:      slog.Make(sloghuman.Sink(os.Stderr)),
		m:        make(map[uuid.UUID]struct{}, 0),
		s:        s,
		tickCh:   nil,
		stopTick: nil,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
		flushCh:  nil,
	}
	for _, opt := range opts {
		opt(hb)
	}
	if hb.tickCh == nil && hb.stopTick == nil {
		ticker := time.NewTicker(DefaultFlushInterval)
		hb.tickCh = ticker.C
		hb.stopTick = ticker.Stop
	}
	return hb
}

type Option func(*Tracker)

// WithLogger sets the logger to be used by Tracker.
func WithLogger(log slog.Logger) Option {
	return func(h *Tracker) {
		h.log = log
	}
}

// WithFlushInterval allows configuring the flush interval of Tracker.
func WithFlushInterval(d time.Duration) Option {
	return func(h *Tracker) {
		ticker := time.NewTicker(d)
		h.tickCh = ticker.C
		h.stopTick = ticker.Stop
	}
}

// WithFlushChannel allows passing a channel that receives
// the number of marked workspaces every time Tracker flushes.
// For testing only.
func WithFlushChannel(c chan int) Option {
	return func(h *Tracker) {
		h.flushCh = c
	}
}

// WithTickChannel allows passing a channel to replace a ticker.
// For testing only.
func WithTickChannel(c chan time.Time) Option {
	return func(h *Tracker) {
		h.tickCh = c
		h.stopTick = func() {}
	}
}

// Add marks the workspace with the given ID as having been used recently.
// Tracker will periodically flush this to its configured Store.
func (wut *Tracker) Add(workspaceID uuid.UUID) {
	wut.mut.Lock()
	wut.m[workspaceID] = struct{}{}
	wut.mut.Unlock()
}

// flushLocked updates last_used_at of all current workspace IDs.
// MUST HOLD LOCK BEFORE CALLING
func (wut *Tracker) flushLocked(now time.Time) {
	if wut.mut.TryLock() {
		panic("developer error: must lock before calling flush()")
	}
	count := len(wut.m)
	defer func() { // only used for testing
		if wut.flushCh != nil {
			wut.flushCh <- count
		}
	}()
	if count == 0 {
		wut.log.Debug(context.Background(), "nothing to flush")
		return
	}
	// Copy our current set of IDs
	ids := make([]uuid.UUID, 0)
	for k := range wut.m {
		ids = append(ids, k)
	}
	// Reset our internal map
	wut.m = make(map[uuid.UUID]struct{})
	// For ease of testing, sort the IDs lexically
	sort.Slice(ids, func(i, j int) bool {
		// For some unfathomable reason, byte arrays are not comparable?
		return strings.Compare(ids[i].String(), ids[j].String()) < 0
	})
	// Set a short-ish timeout for this. We don't want to hang forever.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// nolint: gocritic // system function
	authCtx := dbauthz.AsSystemRestricted(ctx)
	if err := wut.s.BatchUpdateWorkspaceLastUsedAt(authCtx, database.BatchUpdateWorkspaceLastUsedAtParams{
		LastUsedAt: now,
		IDs:        ids,
	}); err != nil {
		wut.log.Error(ctx, "failed updating workspaces last_used_at", slog.F("count", count), slog.Error(err))
		return
	}
	wut.log.Info(ctx, "updated workspaces last_used_at", slog.F("count", count), slog.F("now", now))
}

func (wut *Tracker) Loop() {
	defer func() {
		wut.log.Debug(context.Background(), "workspace usage tracker loop exited")
	}()
	for {
		select {
		case <-wut.stopCh:
			close(wut.doneCh)
			return
		case now, ok := <-wut.tickCh:
			if !ok {
				return
			}
			wut.mut.Lock()
			wut.flushLocked(now.UTC())
			wut.mut.Unlock()
		}
	}
}

// Close stops Tracker and performs a final flush.
func (wut *Tracker) Close() {
	wut.stopOnce.Do(func() {
		wut.stopCh <- struct{}{}
		wut.stopTick()
		<-wut.doneCh
	})
}
