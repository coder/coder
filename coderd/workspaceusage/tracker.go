package workspaceusage

import (
	"bytes"
	"context"
	"flag"
	"os"
	"sort"
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
	log         slog.Logger      // you know, for logs
	flushLock   sync.Mutex       // protects m
	flushErrors int              // tracks the number of consecutive errors flushing
	m           *uuidSet         // stores workspace ids
	s           Store            // for flushing data
	tickCh      <-chan time.Time // controls flush interval
	stopTick    func()           // stops flushing
	stopCh      chan struct{}    // signals us to stop
	stopOnce    sync.Once        // because you only stop once
	doneCh      chan struct{}    // signifies that we have stopped
	flushCh     chan int         // used for testing.
}

// New returns a new Tracker. It is the caller's responsibility
// to call Close().
func New(s Store, opts ...Option) *Tracker {
	tr := &Tracker{
		log:      slog.Make(sloghuman.Sink(os.Stderr)),
		m:        &uuidSet{},
		s:        s,
		tickCh:   nil,
		stopTick: nil,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
		flushCh:  nil,
	}
	for _, opt := range opts {
		opt(tr)
	}
	if tr.tickCh == nil && tr.stopTick == nil {
		tick := time.NewTicker(DefaultFlushInterval)
		tr.tickCh = tick.C
		tr.stopTick = tick.Stop
	}
	go tr.loop()
	return tr
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

// WithTickFlush allows passing two channels: one that reads
// a time.Time, and one that returns the number of marked workspaces
// every time Tracker flushes.
// For testing only and will panic if used outside of tests.
func WithTickFlush(tickCh <-chan time.Time, flushCh chan int) Option {
	if flag.Lookup("test.v") == nil {
		panic("developer error: WithTickFlush is not to be used outside of tests.")
	}
	return func(h *Tracker) {
		h.tickCh = tickCh
		h.stopTick = func() {}
		h.flushCh = flushCh
	}
}

// Add marks the workspace with the given ID as having been used recently.
// Tracker will periodically flush this to its configured Store.
func (tr *Tracker) Add(workspaceID uuid.UUID) {
	tr.m.Add(workspaceID)
}

// flush updates last_used_at of all current workspace IDs.
// If this is held while a previous flush is in progress, it will
// deadlock until the previous flush has completed.
func (tr *Tracker) flush(now time.Time) {
	// Copy our current set of IDs
	ids := tr.m.UniqueAndClear()
	count := len(ids)
	if tr.flushCh != nil { // only used for testing
		defer func() {
			tr.flushCh <- count
		}()
	}
	if count == 0 {
		tr.log.Debug(context.Background(), "nothing to flush")
		return
	}

	// Set a short-ish timeout for this. We don't want to hang forever.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// nolint: gocritic // system function
	authCtx := dbauthz.AsSystemRestricted(ctx)
	tr.flushLock.Lock()
	defer tr.flushLock.Unlock()
	if err := tr.s.BatchUpdateWorkspaceLastUsedAt(authCtx, database.BatchUpdateWorkspaceLastUsedAtParams{
		LastUsedAt: now,
		IDs:        ids,
	}); err != nil {
		// A single failure to flush is likely not a huge problem. If the workspace is still connected at
		// the next iteration, either another coderd instance will likely have this data or the CLI
		// will tell us again that the workspace is in use.
		tr.flushErrors++
		if tr.flushErrors > 1 {
			tr.log.Error(ctx, "multiple failures updating workspaces last_used_at", slog.F("count", count), slog.F("consecutive_errors", tr.flushErrors), slog.Error(err))
			// TODO: if this keeps failing, it indicates a fundamental problem with the database connection.
			// How to surface it correctly to admins besides just screaming into the logs?
		} else {
			tr.log.Warn(ctx, "failed updating workspaces last_used_at", slog.F("count", count), slog.Error(err))
		}
		return
	}
	tr.flushErrors = 0
	tr.log.Info(ctx, "updated workspaces last_used_at", slog.F("count", count), slog.F("now", now))
}

// loop periodically flushes every tick.
// If loop is called after Close, it will exit immediately and log an error.
func (tr *Tracker) loop() {
	select {
	case <-tr.doneCh:
		tr.log.Error(context.Background(), "developer error: Loop called after Close")
		return
	default:
	}
	defer func() {
		close(tr.doneCh)
		tr.log.Debug(context.Background(), "workspace usage tracker loop exited")
	}()
	for {
		select {
		case <-tr.stopCh:
			return
		case now, ok := <-tr.tickCh:
			if !ok {
				return
			}
			// NOTE: we do not update last_used_at with the time at which each workspace was added.
			// Instead, we update with the time of the flush. If the BatchUpdateWorkspacesLastUsedAt
			// query can be rewritten to update each id with a corresponding last_used_at timestamp
			// then we could capture the exact usage time of each workspace. For now however, as
			// we perform this query at a regular interval, the time of the flush is 'close enough'
			// for the purposes of both dormancy (and for autostop, in future).
			tr.flush(now.UTC())
		}
	}
}

// Close stops Tracker and returns once Loop has exited.
// After calling Close(), Loop must not be called.
func (tr *Tracker) Close() error {
	tr.stopOnce.Do(func() {
		tr.stopCh <- struct{}{}
		tr.stopTick()
		<-tr.doneCh
	})
	return nil
}

// uuidSet is a set of UUIDs. Safe for concurrent usage.
// The zero value can be used.
type uuidSet struct {
	l sync.Mutex
	m map[uuid.UUID]struct{}
}

func (s *uuidSet) Add(id uuid.UUID) {
	s.l.Lock()
	defer s.l.Unlock()
	if s.m == nil {
		s.m = make(map[uuid.UUID]struct{})
	}
	s.m[id] = struct{}{}
}

// UniqueAndClear returns the unique set of entries in s and
// resets the internal map.
func (s *uuidSet) UniqueAndClear() []uuid.UUID {
	s.l.Lock()
	defer s.l.Unlock()
	if s.m == nil {
		s.m = make(map[uuid.UUID]struct{})
		return []uuid.UUID{}
	}
	l := make([]uuid.UUID, 0)
	for k := range s.m {
		l = append(l, k)
	}
	// For ease of testing, sort the IDs lexically
	sort.Slice(l, func(i, j int) bool {
		// For some unfathomable reason, byte arrays are not comparable?
		// See https://github.com/golang/go/issues/61004
		return bytes.Compare(l[i][:], l[j][:]) < 0
	})
	clear(s.m)
	return l
}
