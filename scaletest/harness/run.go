package harness
import (
	"fmt"
	"errors"
	"bytes"
	"context"
	"io"
	"sync"
	"time"
)
// Runnable is a test interface that can be executed by a TestHarness.
type Runnable interface {
	// Run should use the passed context to handle cancellation and deadlines
	// properly, and should only return once the test has been fully completed
	// (no lingering goroutines, unless they are cleaned up by the accompanying
	// cleanup function).
	//
	// The test ID (part after the slash) is passed for identification if
	// necessary, and the provided logs write should be used for writing
	// whatever may be necessary for debugging the test.
	Run(ctx context.Context, id string, logs io.Writer) error
}
// Cleanable is an optional extension to Runnable that allows for post-test
// cleanup.
type Cleanable interface {
	Runnable
	// Cleanup should clean up any lingering resources from the test.
	Cleanup(ctx context.Context, id string, logs io.Writer) error
}
// AddRun creates a new *TestRun with the given name, ID and Runnable, adds it
// to the harness and returns it. Panics if the harness has been started, or a
// test with the given run.FullID() is already registered.
//
// This is a convenience method that calls NewTestRun() and h.RegisterRun().
func (h *TestHarness) AddRun(testName string, id string, runner Runnable) *TestRun {
	run := NewTestRun(testName, id, runner)
	h.RegisterRun(run)
	return run
}
// RegisterRun registers the given *TestRun with the harness. Panics if the
// harness has been started, or a test with the given run.FullID() is already
// registered.
func (h *TestHarness) RegisterRun(run *TestRun) {
	h.mut.Lock()
	defer h.mut.Unlock()
	if h.started {
		panic("cannot add a run after the harness has started")
	}
	if _, ok := h.runIDs[run.FullID()]; ok {
		panic("cannot add test with duplicate full ID: " + run.FullID())
	}
	h.runIDs[run.FullID()] = struct{}{}
	h.runs = append(h.runs, run)
}
// TestRun is a single test run and it's accompanying state.
type TestRun struct {
	testName string
	id       string
	runner   Runnable
	logs     *syncBuffer
	done     chan struct{}
	started  time.Time
	duration time.Duration
	err      error
}
func NewTestRun(testName string, id string, runner Runnable) *TestRun {
	return &TestRun{
		testName: testName,
		id:       id,
		runner:   runner,
	}
}
func (r *TestRun) FullID() string {
	return r.testName + "/" + r.id
}
// Run executes the Run function with a self-managed log writer, panic handler,
// error recording and duration recording. The test error is returned.
func (r *TestRun) Run(ctx context.Context) (err error) {
	r.logs = &syncBuffer{
		buf: new(bytes.Buffer),
	}
	r.done = make(chan struct{})
	defer close(r.done)
	r.started = time.Now()
	defer func() {
		r.duration = time.Since(r.started)
		r.err = err
	}()
	defer func() {
		e := recover()
		if e != nil {
			err = fmt.Errorf("panic: %v", e)
		}
	}()
	err = r.runner.Run(ctx, r.id, r.logs)
	//nolint:revive // we use named returns because we mutate it in a defer
	return
}
func (r *TestRun) Cleanup(ctx context.Context) (err error) {
	c, ok := r.runner.(Cleanable)
	if !ok {
		return nil
	}
	select {
	case <-r.done:
	default:
		// Test wasn't executed, so we don't need to clean up.
		return nil
	}
	defer func() {
		e := recover()
		if e != nil {
			err = fmt.Errorf("panic: %v", e)
		}
	}()
	err = c.Cleanup(ctx, r.id, r.logs)
	//nolint:revive // we use named returns because we mutate it in a defer
	return
}
type syncBuffer struct {
	buf *bytes.Buffer
	mut sync.Mutex
}
func (sb *syncBuffer) Write(p []byte) (n int, err error) {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.Write(p)
}
func (sb *syncBuffer) String() string {
	sb.mut.Lock()
	defer sb.mut.Unlock()
	return sb.buf.String()
}
