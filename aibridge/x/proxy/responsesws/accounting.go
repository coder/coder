package responsesws

import (
	"bytes"
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/extract"
	respextract "github.com/coder/coder/v2/aibridge/extract/responses"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/recorder"
)

// Timestamp contract: every record timestamp (StartedAt, CreatedAt,
// EndedAt) is the time of the event that caused the record, captured on the
// goroutine that observed the event: the reader for server frames and the
// session end it reads, Send for client frames and the failures it sees,
// and Close for a close. Jobs and interceptions carry these times to the
// accountant, which never reads the clock for a record, so queueing delay
// never shifts a record.

const (
	// maxQueuedJobs bounds accounting jobs waiting for the accountant. Only
	// events the extractor acts on are queued, a few per response.
	maxQueuedJobs = 256
	// maxQueuedBytes bounds the frame bytes held by queued jobs, since one
	// event can be up to extract.MaxEventBytes. Larger frames are queued
	// without their bytes, so a single event never exceeds it.
	maxQueuedBytes = 16 << 20
)

// errAccountingOverloaded ends an interception that lost an accounting job
// because the queue was full.
var errAccountingOverloaded = xerrors.New("accounting queue overloaded: records of this interception were dropped")

type jobKind int

const (
	// jobStart records the start of an interception the server opened. A
	// failed start is retried before each later job of the interception.
	jobStart jobKind = iota
	// jobEvent feeds a server event to the interception's extraction.
	jobEvent
	// jobBarrier signals done once every earlier job was processed.
	jobBarrier
)

// job is one unit of accounting work, processed by the accountant in queue
// order.
type job struct {
	kind  jobKind
	ic    *interception
	frame []byte // jobEvent
	// oversized is the size of a jobEvent frame over extract.MaxEventBytes,
	// which the extractor skips, so frame is not kept.
	oversized int
	// eventType is the frame's type, as the reader read it. Set for
	// jobEvent.
	eventType string
	// arrived is when the reader read the frame. Records the frame produces
	// and an end it drives carry this time, not the time the accountant
	// processed the frame. Set for jobEvent.
	arrived time.Time
	// terminal marks an event that ends its response: the interception ends
	// after the extraction observed it.
	terminal bool
	done     chan struct{} // jobBarrier
}

// jobQueue is a bounded FIFO. Push never blocks: it accepts all given jobs
// or none.
type jobQueue struct {
	mu       sync.Mutex
	jobs     []job
	bytes    int
	maxJobs  int
	maxBytes int
	// pushed and popped count every job ever pushed and popped, so a
	// position in the queue can be named without holding a job there.
	pushed, popped uint64
	// notify holds a token while jobs may be waiting.
	notify chan struct{}
}

func newJobQueue() *jobQueue {
	return &jobQueue{maxJobs: maxQueuedJobs, maxBytes: maxQueuedBytes, notify: make(chan struct{}, 1)}
}

// push appends jobs if all of them fit, else appends none and returns false.
func (q *jobQueue) push(jobs ...job) bool {
	n := 0
	for _, j := range jobs {
		n += len(j.frame)
	}
	q.mu.Lock()
	if len(q.jobs)+len(jobs) > q.maxJobs || q.bytes+n > q.maxBytes {
		q.mu.Unlock()
		return false
	}
	q.jobs = append(q.jobs, jobs...)
	q.bytes += n
	q.pushed += uint64(len(jobs))
	q.mu.Unlock()
	q.signal()
	return true
}

// pushBarrier appends a barrier regardless of the bounds.
func (q *jobQueue) pushBarrier(done chan struct{}) {
	q.mu.Lock()
	q.jobs = append(q.jobs, job{kind: jobBarrier, done: done})
	q.pushed++
	q.mu.Unlock()
	q.signal()
}

func (q *jobQueue) signal() {
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

func (q *jobQueue) pop() (job, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.jobs) == 0 {
		return job{}, false
	}
	j := q.jobs[0]
	q.jobs[0] = job{}
	q.jobs = q.jobs[1:]
	q.bytes -= len(j.frame)
	q.popped++
	return j, true
}

// counts returns the number of jobs ever pushed and popped.
func (q *jobQueue) counts() (pushed, popped uint64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.pushed, q.popped
}

// accountLoop is the accountant: it calls the extraction methods and records
// what server events report for every interception a job refers to. It
// processes jobs one at a time, so the events of each response are observed
// in order, while the reader keeps forwarding frames. After the reader exits
// it drains the queue and ends every open interception within one shared
// cleanup deadline.
//
// The only other extraction runs in publish, on the Send goroutine, for a
// create whose write raced an error event. That is safe: the create was
// never bound to a response, so no job refers to it and the accountant never
// touches it, and the extraction is created there, so it shares no state
// with the accountant's.
func (s *Session) accountLoop() {
	defer close(s.accountDone)
	for {
		select {
		case <-s.queue.notify:
			s.runJobs()
		case <-s.overloadWake:
			s.runJobs()
		case <-s.readDone:
			s.runJobs()
			s.sweep()
			s.cleanupDeadline.Stop()
			return
		}
	}
}

// runJobs processes queued jobs until the queue is empty, ending each
// overloaded interception once the jobs queued before its hand-off ran.
// Once the cleanup deadline passed, the remaining jobs are dropped with a
// log.
func (s *Session) runJobs() {
	dropped := 0
	for {
		s.endOverloaded()
		j, ok := s.queue.pop()
		if !ok {
			break
		}
		if j.kind != jobBarrier && s.cleanupCtx.Err() != nil {
			dropped++
			s.markLossy(j.ic)
			continue
		}
		s.runJob(j)
	}
	s.endOverloaded()
	if dropped > 0 {
		s.opts.Logger.Error(s.cleanupCtx, "dropped accounting jobs after the cleanup deadline", slog.F("jobs", dropped))
	}
}

func (s *Session) runJob(j job) {
	switch j.kind {
	case jobBarrier:
		close(j.done)
	case jobStart:
		s.ensureStarted(j.ic)
	case jobEvent:
		s.mu.Lock()
		ended := j.ic.ended
		s.mu.Unlock()
		if ended {
			return
		}
		ic := j.ic
		if !s.ensureStarted(ic) {
			// Nothing can be recorded without the interception. A later
			// job retries the start; the terminal event gives up, so the
			// response's mapping never outlives it.
			if j.terminal {
				s.opts.Logger.Warn(s.cleanupCtx, "dropped response: its interception could not be recorded",
					slog.F("interception_id", ic.id), slog.F("response_id", ic.responseID))
				s.forget(ic)
			}
			return
		}
		if ic.ext == nil {
			ic.ext, ic.rec = s.newExtraction(ic)
		}
		ic.rec.at = j.arrived
		if j.oversized > 0 {
			s.opts.Logger.Warn(s.cleanupCtx, "skipped accounting of an event over the size limit",
				slog.F("interception_id", ic.id), slog.F("type", j.eventType),
				slog.F("bytes", j.oversized), slog.F("limit", extract.MaxEventBytes))
		} else {
			ic.ext.OnEvent(j.eventType, j.frame)
		}
		if !j.terminal {
			return
		}
		outcome := ic.ext.Outcome()
		err := outcome.Err
		if outcome.Terminal.Status == extract.TerminalNone {
			s.opts.Logger.Warn(s.cleanupCtx, "extractor skipped a terminal event",
				slog.F("interception_id", ic.id), slog.F("type", j.eventType))
			// The event's type still tells whether the response failed.
			if err == nil && (j.eventType == eventFailed || j.eventType == eventError) {
				err = unparsedFailure(j.eventType)
			}
		}
		s.end(ic, err, j.arrived)
	}
}

// unparsedFailure is the end cause of a response whose failure event the
// extractor skipped, so its error code is unknown. Status 0 categorizes as
// unknown.
func unparsedFailure(eventType string) error {
	return intercept.NewResponseError("upstream "+eventType+" event was not parsed", intercept.OpenAIErrTypeError, "", 0, 0)
}

// overloadedEnd is an interception whose terminal accounting job was
// dropped. It ends once every job queued before the drop ran, so the
// records those jobs carry are kept.
type overloadedEnd struct {
	ic *interception
	// seq is the number of jobs pushed before the drop.
	seq uint64
	// arrived is when the dropped terminal event arrived, and the end time.
	arrived time.Time
}

// handOffLocked hands ic, whose terminal job was dropped, to the accountant
// for an overload end without using the bounded queue, and drops its
// bookkeeping now. Each interception is handed off at most once, since its
// bookkeeping no longer routes frames to it, so the list is bounded by the
// open interceptions. It is called with s.mu held.
func (s *Session) handOffLocked(ic *interception, arrived time.Time) {
	s.forgetLocked(ic)
	pushed, _ := s.queue.counts()
	s.overloaded = append(s.overloaded, overloadedEnd{ic: ic, seq: pushed, arrived: arrived})
	select {
	case s.overloadWake <- struct{}{}:
	default:
	}
}

// endOverloaded ends, with errAccountingOverloaded, every handed-off
// interception whose earlier jobs all ran.
func (s *Session) endOverloaded() {
	_, popped := s.queue.counts()
	s.mu.Lock()
	n := 0
	for n < len(s.overloaded) && s.overloaded[n].seq <= popped {
		n++
	}
	due := slices.Clone(s.overloaded[:n])
	s.overloaded = slices.Delete(s.overloaded, 0, n)
	s.mu.Unlock()
	for _, o := range due {
		if !s.ensureStarted(o.ic) {
			s.opts.Logger.Warn(s.cleanupCtx, "dropped response: its interception could not be recorded",
				slog.F("interception_id", o.ic.id), slog.F("response_id", o.ic.responseID))
			s.forget(o.ic)
			continue
		}
		s.end(o.ic, errAccountingOverloaded, o.arrived)
	}
}

// ensureStarted records the start of ic if it is not recorded yet, and
// reports whether it is. Only the accountant calls it for interceptions the
// server opened; creates are started by Send.
func (s *Session) ensureStarted(ic *interception) bool {
	if ic.started {
		return true
	}
	ctx, cancel := s.recordContext()
	defer cancel()
	if err := s.opts.Recorder.RecordInterception(ctx, s.interceptionRecord(ic, nil)); err != nil {
		s.opts.Logger.Warn(ctx, "failed to record unexplained response", slog.Error(err),
			slog.F("interception_id", ic.id), slog.F("response_id", ic.responseID))
		return false
	}
	ic.started = true
	return true
}

// forget drops ic without an end record, for an interception whose start
// was never recorded.
func (s *Session) forget(ic *interception) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.markEndedLocked(ic)
	s.forgetLocked(ic)
}

// sweep ends every open interception with the session's cause, or with
// errAccountingOverloaded when accounting dropped one of its jobs.
func (s *Session) sweep() {
	type ending struct {
		ic  *interception
		err error
	}
	s.mu.Lock()
	cause, endedAt := s.endErr, s.endedAt
	if cause == nil {
		cause = ErrClosed
	}
	var ended []ending
	open := slices.Collect(maps.Values(s.open))
	for _, o := range s.overloaded {
		open = append(open, o.ic)
	}
	for _, ic := range open {
		if !s.markEndedLocked(ic) {
			continue
		}
		err := cause
		if ic.lossy {
			err = errAccountingOverloaded
		}
		ended = append(ended, ending{ic, err})
	}
	clear(s.open)
	s.overloaded = nil
	clear(s.pending)
	clear(s.active)
	clear(s.responses)
	clear(s.toolCalls)
	s.mu.Unlock()
	for _, e := range ended {
		if !e.ic.started {
			s.opts.Logger.Warn(s.cleanupCtx, "dropped response: its interception could not be recorded",
				slog.F("interception_id", e.ic.id), slog.F("response_id", e.ic.responseID))
			continue
		}
		s.recordEnded(e.ic, e.err, endedAt)
	}
}

// pushLocked queues jobs for ic. On overload it drops them, logs the type of
// the last one, and marks ic so its end reports the loss. It is called with
// s.mu held.
func (s *Session) pushLocked(ic *interception, jobs ...job) bool {
	// Queued frames are copied: the reader hands its frame to the client,
	// which owns it and may modify it before the accountant runs. A frame
	// the extractor would skip is not kept; the reader already took the IDs
	// its job needs, and a terminal event's type still decides the outcome.
	for i := range jobs {
		switch {
		case len(jobs[i].frame) > extract.MaxEventBytes:
			jobs[i].oversized = len(jobs[i].frame)
			jobs[i].frame = nil
		case jobs[i].frame != nil:
			jobs[i].frame = bytes.Clone(jobs[i].frame)
		}
	}
	if s.queue.push(jobs...) {
		return true
	}
	ic.lossy = true
	s.opts.Logger.Error(s.cleanupCtx, "accounting queue full: dropped event",
		slog.F("interception_id", ic.id), slog.F("response_id", ic.responseID), slog.F("type", jobs[len(jobs)-1].eventType))
	return false
}

// byteBudget bounds the bytes held by a buffer with one producer: the
// producer acquires a frame's bytes before buffering it and the consumer
// releases them once it took the frame.
type byteBudget struct {
	mu    sync.Mutex
	used  int
	limit int
	// freed holds a token after a release, so a waiting producer rechecks.
	freed chan struct{}
}

func newByteBudget(maxBytes int) *byteBudget {
	return &byteBudget{limit: maxBytes, freed: make(chan struct{}, 1)}
}

// acquire waits until n more bytes fit, or nothing is held so any frame
// fits, and reports false if ctx ended first.
func (b *byteBudget) acquire(ctx context.Context, n int) bool {
	for {
		b.mu.Lock()
		if b.used == 0 || b.used+n <= b.limit {
			b.used += n
			b.mu.Unlock()
			return true
		}
		b.mu.Unlock()
		select {
		case <-b.freed:
		case <-ctx.Done():
			return false
		}
	}
}

func (b *byteBudget) release(n int) {
	b.mu.Lock()
	b.used -= n
	b.mu.Unlock()
	select {
	case b.freed <- struct{}{}:
	default:
	}
}

func (s *Session) markLossy(ic *interception) {
	if ic == nil {
		return
	}
	s.mu.Lock()
	ic.lossy = true
	s.mu.Unlock()
}

// newExtraction returns the extraction recording ic's response and its
// recorder. Its recorder calls are bounded per call and by the cleanup
// deadline.
func (s *Session) newExtraction(ic *interception) (*respextract.ResponseExtraction, *boundedRecorder) {
	rec := &boundedRecorder{Recorder: s.opts.Recorder}
	return respextract.NewResponseExtraction(s.cleanupCtx, s.opts.Logger, rec, ic.id, ic.prompt), rec
}

// recordContext bounds one record call: it is detached from caller and
// session cancellation, lasts at most recorder.DefaultAsyncTimeout, and ends
// at the shutdown cleanup deadline.
func (s *Session) recordContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(s.cleanupCtx, recorder.DefaultAsyncTimeout)
}

// boundedRecorder gives each record call of an extraction its own timeout,
// since extractions scope all calls with one context, and stamps records
// with the arrival time of the frame being extracted.
type boundedRecorder struct {
	recorder.Recorder
	// at, when set, is the arrival time of the frame being extracted. It
	// replaces the CreatedAt of every record, since the accountant may
	// process a frame long after it arrived. Only the accountant sets it,
	// before each OnEvent call.
	at time.Time
}

func (r *boundedRecorder) stamp(createdAt *time.Time) {
	if !r.at.IsZero() {
		*createdAt = r.at.UTC()
	}
}

func (r *boundedRecorder) RecordPromptUsage(ctx context.Context, req *recorder.PromptUsageRecord) error {
	r.stamp(&req.CreatedAt)
	ctx, cancel := context.WithTimeout(ctx, recorder.DefaultAsyncTimeout)
	defer cancel()
	return r.Recorder.RecordPromptUsage(ctx, req)
}

func (r *boundedRecorder) RecordTokenUsage(ctx context.Context, req *recorder.TokenUsageRecord) error {
	r.stamp(&req.CreatedAt)
	ctx, cancel := context.WithTimeout(ctx, recorder.DefaultAsyncTimeout)
	defer cancel()
	return r.Recorder.RecordTokenUsage(ctx, req)
}

func (r *boundedRecorder) RecordToolUsage(ctx context.Context, req *recorder.ToolUsageRecord) error {
	r.stamp(&req.CreatedAt)
	ctx, cancel := context.WithTimeout(ctx, recorder.DefaultAsyncTimeout)
	defer cancel()
	return r.Recorder.RecordToolUsage(ctx, req)
}

func (r *boundedRecorder) RecordModelThought(ctx context.Context, req *recorder.ModelThoughtRecord) error {
	r.stamp(&req.CreatedAt)
	ctx, cancel := context.WithTimeout(ctx, recorder.DefaultAsyncTimeout)
	defer cancel()
	return r.Recorder.RecordModelThought(ctx, req)
}
