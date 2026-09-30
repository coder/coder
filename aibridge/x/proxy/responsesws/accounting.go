package responsesws

import (
	"context"
	"sync"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/extract"
	respextract "github.com/coder/coder/v2/aibridge/extract/responses"
	"github.com/coder/coder/v2/aibridge/recorder"
)

const (
	// maxQueuedJobs bounds accounting jobs waiting for the accountant. Only
	// events the extractor acts on are queued, a few per response.
	maxQueuedJobs = 256
	// maxQueuedBytes bounds the frame bytes held by queued jobs, since one
	// event can be up to extract.MaxEventBytes.
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
	q.mu.Unlock()
	q.signal()
	return true
}

// pushBarrier appends a barrier regardless of the bounds.
func (q *jobQueue) pushBarrier(done chan struct{}) {
	q.mu.Lock()
	q.jobs = append(q.jobs, job{kind: jobBarrier, done: done})
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
	return j, true
}

// accountLoop is the accountant: the only goroutine that calls extraction
// methods or records what server events report. It processes jobs one at a
// time, so the events of each response are observed in order, while the
// reader keeps forwarding frames. After the reader exits it drains the queue
// and ends every open interception within one shared cleanup deadline.
func (s *Session) accountLoop() {
	defer close(s.accountDone)
	for {
		select {
		case <-s.queue.notify:
			s.runJobs()
		case <-s.readDone:
			s.runJobs()
			s.sweep()
			s.cleanupDeadline.Stop()
			return
		}
	}
}

// runJobs processes queued jobs until the queue is empty. Once the cleanup
// deadline passed, the remaining jobs are dropped with a log.
func (s *Session) runJobs() {
	dropped := 0
	for {
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
			ic.ext = s.newExtraction(ic)
		}
		ic.ext.OnEvent("", j.frame)
		if !j.terminal {
			return
		}
		outcome := ic.ext.Outcome()
		if outcome.Terminal.Status == extract.TerminalNone {
			s.opts.Logger.Warn(s.cleanupCtx, "extractor skipped a terminal event", slog.F("interception_id", ic.id))
		}
		s.end(ic, outcome.Err)
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
	ic.ended = true
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
	cause := s.endErr
	if cause == nil {
		cause = ErrClosed
	}
	var ended []ending
	for _, ic := range s.open {
		if ic.ended {
			continue
		}
		ic.ended = true
		err := cause
		if ic.lossy {
			err = errAccountingOverloaded
		}
		ended = append(ended, ending{ic, err})
	}
	clear(s.open)
	clear(s.pending)
	clear(s.active)
	clear(s.responses)
	s.mu.Unlock()
	for _, e := range ended {
		if !e.ic.started {
			s.opts.Logger.Warn(s.cleanupCtx, "dropped response: its interception could not be recorded",
				slog.F("interception_id", e.ic.id), slog.F("response_id", e.ic.responseID))
			continue
		}
		s.recordEnded(e.ic, e.err)
	}
}

// pushLocked queues jobs for ic. On overload it drops them, logs, and marks
// ic so its end reports the loss. It is called with s.mu held.
func (s *Session) pushLocked(ic *interception, eventType string, jobs ...job) bool {
	if s.queue.push(jobs...) {
		return true
	}
	ic.lossy = true
	s.opts.Logger.Error(s.cleanupCtx, "accounting queue full: dropped event",
		slog.F("interception_id", ic.id), slog.F("response_id", ic.responseID), slog.F("type", eventType))
	return false
}

func (s *Session) markLossy(ic *interception) {
	if ic == nil {
		return
	}
	s.mu.Lock()
	ic.lossy = true
	s.mu.Unlock()
}

// newExtraction returns the extraction recording ic's response. Its
// recorder calls are bounded per call and by the cleanup deadline.
func (s *Session) newExtraction(ic *interception) *respextract.ResponseExtraction {
	return respextract.NewResponseExtraction(s.cleanupCtx, s.opts.Logger, boundedRecorder{s.opts.Recorder}, ic.id, ic.prompt)
}

// recordContext bounds one record call: it is detached from caller and
// session cancellation, lasts at most recorder.DefaultAsyncTimeout, and ends
// at the shutdown cleanup deadline.
func (s *Session) recordContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(s.cleanupCtx, recorder.DefaultAsyncTimeout)
}

// boundedRecorder gives each record call of an extraction its own timeout,
// since extractions scope all calls with one context.
type boundedRecorder struct {
	recorder.Recorder
}

func (r boundedRecorder) RecordPromptUsage(ctx context.Context, req *recorder.PromptUsageRecord) error {
	ctx, cancel := context.WithTimeout(ctx, recorder.DefaultAsyncTimeout)
	defer cancel()
	return r.Recorder.RecordPromptUsage(ctx, req)
}

func (r boundedRecorder) RecordTokenUsage(ctx context.Context, req *recorder.TokenUsageRecord) error {
	ctx, cancel := context.WithTimeout(ctx, recorder.DefaultAsyncTimeout)
	defer cancel()
	return r.Recorder.RecordTokenUsage(ctx, req)
}

func (r boundedRecorder) RecordToolUsage(ctx context.Context, req *recorder.ToolUsageRecord) error {
	ctx, cancel := context.WithTimeout(ctx, recorder.DefaultAsyncTimeout)
	defer cancel()
	return r.Recorder.RecordToolUsage(ctx, req)
}

func (r boundedRecorder) RecordModelThought(ctx context.Context, req *recorder.ModelThoughtRecord) error {
	ctx, cancel := context.WithTimeout(ctx, recorder.DefaultAsyncTimeout)
	defer cancel()
	return r.Recorder.RecordModelThought(ctx, req)
}
