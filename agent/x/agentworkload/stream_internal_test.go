//go:build linux

package agentworkload

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func streamServer(t *testing.T, s *Supervisor) (*httptest.Server, <-chan struct{}) {
	t.Helper()
	done := make(chan struct{}, 16)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { done <- struct{}{} }()
		s.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return server, done
}

func openOutputStream(t *testing.T, server *httptest.Server, s *Supervisor, id uuid.UUID, offset int64) (*http.Response, *bufio.Reader) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/workloads/"+id.String()+"?follow=true&offset="+strconv.FormatInt(offset, 10), nil)
	require.NoError(t, err)
	r.Header.Set("Authorization", "Bearer "+s.config.Token)
	response, err := server.Client().Do(r)
	require.NoError(t, err)
	t.Cleanup(func() { _ = response.Body.Close() })
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, "application/x-ndjson", response.Header.Get("Content-Type"))
	require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
	require.Equal(t, "no", response.Header.Get("X-Accel-Buffering"))
	return response, bufio.NewReader(response.Body)
}

func readOutputBatch(t *testing.T, reader *bufio.Reader) codersdk.WorkloadOutput {
	t.Helper()
	line, err := reader.ReadBytes('\n')
	require.NoError(t, err)
	var output codersdk.WorkloadOutput
	require.NoError(t, json.Unmarshal(line, &output))
	return output
}

func requireStreamEOF(t *testing.T, reader *bufio.Reader) {
	t.Helper()
	line, err := reader.ReadBytes('\n')
	require.ErrorIs(t, err, io.EOF)
	require.Empty(t, line)
}

func TestStreamOutputLiveResume(t *testing.T) {
	t.Parallel()
	rt := newFake()
	attachment, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close() })
	rt.attachment = attachment
	s := testSupervisor(t, testConfig(t), rt)
	s.clock = quartz.NewMock(t)
	req := testRequest()
	require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
	receive(t, rt.waiting)
	server, detached := streamServer(t, s)
	response, reader := openOutputStream(t, server, s, req.ID, 0)
	initial := readOutputBatch(t, reader)
	require.Equal(t, "running", initial.Workload.State)
	require.Empty(t, initial.Events)

	_, err := writer.Write(frame(1, []byte("before exit")))
	require.NoError(t, err)
	first := readOutputBatch(t, reader)
	require.Equal(t, "running", first.Workload.State)
	require.Equal(t, []codersdk.WorkloadEvent{{Stream: "stdout", Data: []byte("before exit")}}, first.Events)
	require.Equal(t, int64(1), first.NextOffset)

	require.NoError(t, response.Body.Close())
	receive(t, detached)
	s.mu.Lock()
	run := s.state.Runs[req.ID]
	s.mu.Unlock()
	require.False(t, s.canceled(run), "disconnect must not cancel execution")
	_, _, removes := rt.counts()
	require.Zero(t, removes)

	_, err = writer.Write(frame(2, []byte("after reconnect")))
	require.NoError(t, err)
	_, reader = openOutputStream(t, server, s, req.ID, first.NextOffset)
	var events []codersdk.WorkloadEvent
	for len(events) == 0 {
		batch := readOutputBatch(t, reader)
		require.Equal(t, "running", batch.Workload.State)
		events = append(events, batch.Events...)
	}
	require.Equal(t, []codersdk.WorkloadEvent{{Stream: "stderr", Data: []byte("after reconnect")}}, events)
	// Exit can precede the attachment's final bytes. Completion must wait for
	// those bytes, and resuming must neither duplicate output nor replay work.
	rt.finish <- 7
	_, err = writer.Write(frame(1, []byte("trailing")))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	for {
		batch := readOutputBatch(t, reader)
		events = append(events, batch.Events...)
		if batch.Workload.Terminal() && len(batch.Events) == 0 {
			require.Equal(t, "failed", batch.Workload.State)
			require.Equal(t, new(7), batch.Workload.ExitCode)
			require.Equal(t, int64(3), batch.NextOffset)
			break
		}
	}
	require.Equal(t, []codersdk.WorkloadEvent{{Stream: "stderr", Data: []byte("after reconnect")}, {Stream: "stdout", Data: []byte("trailing")}}, events)
	requireStreamEOF(t, reader)
	joinRuns(t, s)
	creates, starts, removes := rt.counts()
	require.Equal(t, 1, creates)
	require.Equal(t, 1, starts)
	require.Equal(t, 1, removes)
}

func TestStreamOutputTerminalDrain(t *testing.T) {
	t.Parallel()
	for _, bound := range []string{"bytes", "events"} {
		t.Run(bound, func(t *testing.T) {
			t.Parallel()
			rt := newFake()
			count, size := 2*maxBatchEvents+1, 1
			if bound == "bytes" {
				size = 16 << 10
				count = 2*maxBatchBytes/size + 1
			}
			for range count {
				rt.output = append(rt.output, frame(1, bytes.Repeat([]byte("x"), size))...)
			}
			rt.finish <- 0
			s := testSupervisor(t, testConfig(t), rt)
			s.clock = quartz.NewMock(t)
			req := testRequest()
			require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
			joinRuns(t, s)
			server, _ := streamServer(t, s)
			_, reader := openOutputStream(t, server, s, req.ID, 0)
			var offset int64
			batches := 0
			for {
				batch := readOutputBatch(t, reader)
				require.True(t, batch.Workload.Terminal())
				require.LessOrEqual(t, len(batch.Events), maxBatchEvents)
				require.LessOrEqual(t, len(batch.Events)*size, maxBatchBytes)
				offset += int64(len(batch.Events))
				require.Equal(t, offset, batch.NextOffset)
				if len(batch.Events) == 0 {
					break
				}
				batches++
			}
			require.Equal(t, int64(count), offset)
			require.Equal(t, 3, batches)
			requireStreamEOF(t, reader)
			// The non-follow API remains a bounded JSON snapshot.
			snapshot := getOutput(t, s, req.ID, "0")
			require.Less(t, len(snapshot.Events), count)
		})
	}
}

func TestStreamOutputIdleHeartbeatTruncationAndClose(t *testing.T) {
	t.Parallel()
	clock := quartz.NewMock(t)
	rt := newFake()
	s := testSupervisor(t, testConfig(t), rt)
	s.clock = clock
	req := testRequest()
	require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
	receive(t, rt.waiting)
	server, detached := streamServer(t, s)
	_, reader := openOutputStream(t, server, s, req.ID, 0)
	require.Empty(t, readOutputBatch(t, reader).Events)
	s.mu.Lock()
	run := s.state.Runs[req.ID]
	s.mu.Unlock()
	ctx := testutil.Context(t, testutil.WaitLong)
	clock.Advance(14 * time.Second).MustWait(ctx)
	s.appendOutput(run, "stdout", []byte("live"))
	require.Len(t, readOutputBatch(t, reader).Events, 1, "idle time must not produce repeated snapshots")
	for _, advance := range []time.Duration{time.Second, 15 * time.Second, 15 * time.Second} {
		clock.Advance(advance).MustWait(ctx)
		line, err := reader.ReadString('\n')
		require.NoError(t, err)
		require.Equal(t, "\n", line, "heartbeat must be a blank line, not a snapshot")
	}
	s.truncate(run)
	truncated := readOutputBatch(t, reader)
	require.True(t, truncated.Truncated)
	require.Empty(t, truncated.Events)
	s.truncate(run)
	clock.Advance(outputHeartbeatInterval).MustWait(ctx)
	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "\n", line, "unchanged truncation must not notify again")
	require.NoError(t, s.Close())
	receive(t, detached)
	requireStreamEOF(t, reader)
	_, _, removes := rt.counts()
	require.Zero(t, removes)
}

type recordedOutputWrite struct {
	deadline      time.Time
	writtenAt     time.Time
	deadlineCalls int
}

type deadlineRecordingWriter struct {
	http.ResponseWriter
	deadline      time.Time
	deadlineCalls int
	writes        chan<- recordedOutputWrite
}

func (w *deadlineRecordingWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	w.deadlineCalls++
	return http.NewResponseController(w.ResponseWriter).SetWriteDeadline(deadline)
}

func (w *deadlineRecordingWriter) Write(data []byte) (int, error) {
	w.writes <- recordedOutputWrite{deadline: w.deadline, writtenAt: time.Now(), deadlineCalls: w.deadlineCalls}
	return w.ResponseWriter.Write(data)
}

func (w *deadlineRecordingWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func TestStreamOutputRefreshesWriteDeadline(t *testing.T) {
	t.Parallel()
	clock := quartz.NewMock(t)
	rt := newFake()
	s := testSupervisor(t, testConfig(t), rt)
	s.clock = clock
	req := testRequest()
	require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
	receive(t, rt.waiting)
	writes := make(chan recordedOutputWrite, 16)
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		s.ServeHTTP(&deadlineRecordingWriter{ResponseWriter: w, writes: writes}, r)
	}))
	t.Cleanup(server.Close)
	ctx := testutil.Context(t, testutil.WaitLong)
	calls := 0
	assertDeadline := func(before time.Time) {
		t.Helper()
		write := testutil.RequireReceive(ctx, t, writes)
		calls++
		require.Equal(t, calls, write.deadlineCalls, "each write must refresh its deadline before writing")
		// Network deadlines use wall time, even when heartbeat scheduling uses
		// Quartz. Verify the timeout is bounded and starts with this write.
		require.False(t, write.deadline.Before(before.Add(operationTimeout)))
		require.False(t, write.deadline.After(write.writtenAt.Add(operationTimeout)))
	}

	before := time.Now()
	response, reader := openOutputStream(t, server, s, req.ID, 0)
	require.Empty(t, readOutputBatch(t, reader).Events)
	assertDeadline(before)
	for range 5 {
		before = time.Now()
		clock.Advance(outputHeartbeatInterval).MustWait(ctx)
		line, err := reader.ReadString('\n')
		require.NoError(t, err)
		require.Equal(t, "\n", line)
		assertDeadline(before)
	}
	s.mu.Lock()
	run := s.state.Runs[req.ID]
	s.mu.Unlock()
	before = time.Now()
	s.appendOutput(run, "stdout", []byte("after heartbeats"))
	require.Len(t, readOutputBatch(t, reader).Events, 1)
	assertDeadline(before)
	require.NoError(t, response.Body.Close())
	receive(t, done)
}

func TestStreamOutputValidation(t *testing.T) {
	t.Parallel()
	s := testSupervisor(t, testConfig(t), newFake())
	req := testRequest()
	require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
	for _, tc := range []struct {
		name, path, token string
		status            int
	}{
		{name: "Negative", path: req.ID.String() + "?offset=-1", token: s.config.Token, status: http.StatusBadRequest},
		{name: "NonInteger", path: req.ID.String() + "?offset=no", token: s.config.Token, status: http.StatusBadRequest},
		{name: "Overflow", path: req.ID.String() + "?offset=999999999999999999999", token: s.config.Token, status: http.StatusBadRequest},
		{name: "BeyondEnd", path: req.ID.String() + "?offset=1", token: s.config.Token, status: http.StatusBadRequest},
		{name: "MissingRun", path: uuid.NewString() + "?offset=0", token: s.config.Token, status: http.StatusNotFound},
		{name: "MissingAuth", path: req.ID.String() + "?offset=0", status: http.StatusUnauthorized},
		{name: "InvalidAuth", path: req.ID.String() + "?offset=0", token: "wrong", status: http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(http.MethodGet, "/workloads/"+tc.path+"&follow=true", nil)
			r.Header.Set("Authorization", "Bearer "+tc.token)
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, "application/json", w.Header().Get("Content-Type"))
			var response codersdk.Response
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			require.NotEmpty(t, response.Message)
		})
	}
}

func TestStreamOutputTerminalWakeups(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"unknown", "output-persistence", "state-persistence"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			cfg := testConfig(t)
			rt := newFake()
			gate := make(chan struct{})
			if failure == "unknown" {
				rt.createGate = gate
				rt.createErr = xerrors.New("unknown creation outcome")
			}
			s := testSupervisor(t, cfg, rt)
			s.clock = quartz.NewMock(t)
			req := testRequest()
			require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
			if failure == "unknown" {
				receive(t, rt.created)
			} else {
				receive(t, rt.waiting)
			}
			server, _ := streamServer(t, s)
			_, reader := openOutputStream(t, server, s, req.ID, 0)
			require.False(t, readOutputBatch(t, reader).Workload.Terminal())
			switch failure {
			case "unknown":
				close(gate)
			case "output-persistence":
				require.NoError(t, os.Mkdir(filepath.Join(cfg.StateDir, outputName(req.ID)), 0o700))
				rt.finish <- 0
			case "state-persistence":
				_ = blockStatePersistence(t, cfg)
				rt.finish <- 0
			}
			for {
				batch := readOutputBatch(t, reader)
				if !batch.Workload.Terminal() {
					continue
				}
				require.Empty(t, batch.Events)
				require.True(t, batch.Workload.CleanupPending)
				if failure == "unknown" {
					require.Equal(t, "unknown", batch.Workload.State)
				} else {
					require.Equal(t, "succeeded", batch.Workload.State)
				}
				break
			}
			requireStreamEOF(t, reader)
			joinRuns(t, s)
		})
	}
}

// Closing a failed attachment can still release its buffered trailing output.
// This reader makes that race deterministic instead of relying on scheduling.
type trailingAttachment struct {
	reader *bytes.Reader
	closed chan struct{}
	once   sync.Once
}

func (r *trailingAttachment) Read(p []byte) (int, error) {
	<-r.closed
	return r.reader.Read(p)
}

func (r *trailingAttachment) Close() error {
	r.once.Do(func() { close(r.closed) })
	return nil
}

func TestStreamOutputUnknownDrainsAttachment(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"start", "wait", "running-persistence"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			cfg := testConfig(t)
			rt := newFake()
			gate := make(chan struct{})
			rt.createGate = gate
			rt.attachment = &trailingAttachment{
				reader: bytes.NewReader(frame(1, []byte("trailing output"))),
				closed: make(chan struct{}),
			}
			switch phase {
			case "start":
				rt.startErr = xerrors.New("unknown start")
			case "wait":
				rt.waitErr = xerrors.New("unknown exit")
			}
			s := testSupervisor(t, cfg, rt)
			s.clock = quartz.NewMock(t)
			req := testRequest()
			require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
			receive(t, rt.created)
			server, _ := streamServer(t, s)
			_, reader := openOutputStream(t, server, s, req.ID, 0)
			require.Equal(t, "requested", readOutputBatch(t, reader).Workload.State)
			if phase == "running-persistence" {
				_ = blockStatePersistence(t, cfg)
			}
			close(gate)
			var events []codersdk.WorkloadEvent
			for {
				batch := readOutputBatch(t, reader)
				events = append(events, batch.Events...)
				if batch.Workload.Terminal() && len(batch.Events) == 0 {
					require.Equal(t, "unknown", batch.Workload.State)
					break
				}
			}
			require.Equal(t, []codersdk.WorkloadEvent{{Stream: "stdout", Data: []byte("trailing output")}}, events)
			requireStreamEOF(t, reader)
			joinRuns(t, s)
			_, _, removes := rt.counts()
			require.Zero(t, removes)
		})
	}
}

func TestStreamOutputReadFailure(t *testing.T) {
	t.Parallel()
	s := testSupervisor(t, testConfig(t), newFake())
	s.clock = quartz.NewMock(t)
	req := testRequest()
	// A synthetic record permits invalidating retained output independently of
	// execution, without timing a filesystem failure against a running command.
	run := &record{Request: req, Workload: codersdk.Workload{ID: req.ID, State: "running"}}
	s.state.Runs[req.ID] = run
	server, detached := streamServer(t, s)
	_, reader := openOutputStream(t, server, s, req.ID, 0)
	require.Empty(t, readOutputBatch(t, reader).Events)
	s.mu.Lock()
	run.outputOnDisk = true
	s.notifyLocked(run)
	s.mu.Unlock()
	// After committing NDJSON, a read error closes the response without an
	// error document or a false terminal marker.
	receive(t, detached)
	requireStreamEOF(t, reader)
	// Before committing the response, the same error is an ordinary HTTP error.
	w := requestHTTP(t, s, http.MethodGet, "/workloads/"+req.ID.String()+"?follow=true", nil)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))
}

// The first flush pauses after sending the snapshot, allowing output to arrive
// exactly between the handler's read and its wait on the notification channel.
type pausedFlushWriter struct {
	http.ResponseWriter
	ctx     context.Context
	flushed chan struct{}
	resume  <-chan struct{}
}

func (w *pausedFlushWriter) Flush() {
	_ = http.NewResponseController(w.ResponseWriter).Flush()
	if w.flushed != nil {
		close(w.flushed)
		w.flushed = nil
		select {
		case <-w.resume:
		case <-w.ctx.Done():
		}
	}
}

func TestStreamOutputReadSubscribeBoundary(t *testing.T) {
	t.Parallel()
	rt := newFake()
	s := testSupervisor(t, testConfig(t), rt)
	s.clock = quartz.NewMock(t)
	req := testRequest()
	require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
	receive(t, rt.waiting)
	flushed, resume := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		s.ServeHTTP(&pausedFlushWriter{ResponseWriter: w, ctx: ctx, flushed: flushed, resume: resume}, r.WithContext(ctx))
	}))
	t.Cleanup(server.Close)
	t.Cleanup(cancel)
	_, reader := openOutputStream(t, server, s, req.ID, 0)
	require.Empty(t, readOutputBatch(t, reader).Events)
	receive(t, flushed)
	appended := make(chan struct{})
	go func() {
		s.mu.Lock()
		run := s.state.Runs[req.ID]
		s.mu.Unlock()
		s.appendOutput(run, "stdout", []byte("at the boundary"))
		close(appended)
	}()
	receive(t, appended)
	close(resume)
	batch := readOutputBatch(t, reader)
	require.Equal(t, []codersdk.WorkloadEvent{{Stream: "stdout", Data: []byte("at the boundary")}}, batch.Events)
	cancel()
	receive(t, done)
	requireStreamEOF(t, reader)
}
