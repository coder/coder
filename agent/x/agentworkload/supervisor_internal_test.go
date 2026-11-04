//go:build linux

package agentworkload

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

const testImageID = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type fakeRuntime struct {
	mu                                      sync.Mutex
	creates, starts, removes                int
	createErr, startErr, waitErr, removeErr error
	createGate                              <-chan struct{}
	output                                  []byte
	attachment                              io.ReadCloser
	outputObserved                          chan struct{}
	created, started, waiting               chan struct{}
	finish                                  chan int
	onCreate                                func(codersdk.StartHostWorkloadRequest) error
	executionDuration                       time.Duration
	executionDurationErr                    error
	onExecutionDuration                     func(context.Context)
}

func newFake() *fakeRuntime {
	return &fakeRuntime{created: make(chan struct{}, maxHistory), started: make(chan struct{}, maxHistory), waiting: make(chan struct{}, maxHistory), finish: make(chan int, maxHistory)}
}
func (*fakeRuntime) Validate(context.Context) (string, error) { return testImageID, nil }
func (f *fakeRuntime) Create(ctx context.Context, _ string, req codersdk.StartHostWorkloadRequest) error {
	f.mu.Lock()
	f.creates++
	f.mu.Unlock()
	f.created <- struct{}{}
	if f.onCreate != nil {
		if err := f.onCreate(req); err != nil {
			return err
		}
	}
	if f.createGate != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-f.createGate:
		}
	}
	return f.createErr
}

type observedReader struct {
	io.Reader
	observed chan struct{}
}

func (r *observedReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF && r.observed != nil {
		close(r.observed)
		r.observed = nil
	}
	return n, err
}

func (f *fakeRuntime) Attach(context.Context, string) (io.ReadCloser, error) {
	if f.attachment != nil {
		return f.attachment, nil
	}
	return io.NopCloser(&observedReader{Reader: bytes.NewReader(f.output), observed: f.outputObserved}), nil
}

func (f *fakeRuntime) Start(context.Context, string) error {
	f.mu.Lock()
	f.starts++
	f.mu.Unlock()
	f.started <- struct{}{}
	return f.startErr
}

func (f *fakeRuntime) Wait(ctx context.Context, _ string) (int, error) {
	f.waiting <- struct{}{}
	if f.waitErr != nil {
		return 0, f.waitErr
	}
	select {
	case code := <-f.finish:
		return code, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (f *fakeRuntime) ExecutionDuration(ctx context.Context, _ string) (time.Duration, error) {
	if f.onExecutionDuration != nil {
		f.onExecutionDuration(ctx)
	}
	return f.executionDuration, f.executionDurationErr
}

func (f *fakeRuntime) Remove(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removes++
	return f.removeErr
}
func (*fakeRuntime) Close() error { return nil }
func (f *fakeRuntime) counts() (creates, starts, removes int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.creates, f.starts, f.removes
}

func testConfig(t *testing.T) Config {
	t.Helper()
	return Config{Token: "0123456789abcdef0123456789abcdef", StateDir: filepath.Join(t.TempDir(), "state"), Image: "prepared:latest", Workdir: "/repo", CPU: 2, MemoryBytes: 512 << 20, Repositories: map[string]string{"repo": strings.Repeat("a", 40)}}
}

func testSupervisor(t *testing.T, cfg Config, rt workloadRuntime) *Supervisor {
	t.Helper()
	s, err := newSupervisor(t.Context(), cfg, rt)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	return s
}

func testRequest() codersdk.StartHostWorkloadRequest {
	return codersdk.StartHostWorkloadRequest{
		CreateWorkloadRequest: codersdk.CreateWorkloadRequest{ID: uuid.New(), TemplateID: uuid.New(), CPU: 1, MemoryBytes: 128 << 20, Command: "echo sensitive-command"},
		OrganizationID:        uuid.New(), OwnerID: uuid.New(), GenerationID: testImageID,
	}
}

func requestHTTP(t *testing.T, s *Supervisor, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(data)
	}
	r := httptest.NewRequest(method, path, reader)
	r.Header.Set("Authorization", "Bearer "+s.config.Token)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func receive(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(testutil.WaitLong):
		t.Fatal("timed out waiting for runtime")
	}
}

func joinRuns(t *testing.T, s *Supervisor) {
	t.Helper()
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	receive(t, done)
}

func frame(stream byte, data []byte) []byte {
	header := make([]byte, 8)
	header[0] = stream
	//nolint:gosec // Test payloads are bounded well below the uint32 limit.
	binary.BigEndian.PutUint32(header[4:], uint32(len(data)))
	//nolint:makezero // The payload follows the initialized eight-byte header.
	return append(header, data...)
}

func getOutput(t *testing.T, s *Supervisor, id uuid.UUID, offset string) codersdk.WorkloadOutput {
	t.Helper()
	w := requestHTTP(t, s, http.MethodGet, "/workloads/"+id.String()+"?offset="+offset, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var out codersdk.WorkloadOutput
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return out
}

func TestLifecycle(t *testing.T) {
	t.Parallel()
	clock := quartz.NewMock(t)
	const provisionDuration = 123*time.Millisecond + 456*time.Nanosecond
	const executionDuration = 2*time.Second + 789*time.Nanosecond
	cfg := testConfig(t)
	rt := newFake()
	rt.executionDuration = executionDuration
	rt.output = append(frame(1, []byte("hello")), frame(2, []byte("error"))...)
	rt.outputObserved = make(chan struct{})
	persisted := make(chan struct{}, 1)
	rt.onCreate = func(req codersdk.StartHostWorkloadRequest) error {
		data, err := os.ReadFile(filepath.Join(cfg.StateDir, "state.json"))
		if err != nil {
			return err
		}
		var state durableState
		if err := json.Unmarshal(data, &state); err != nil {
			return err
		}
		if run := state.Runs[req.ID]; run == nil || !reflect.DeepEqual(run.Request, req) || !run.Reserved || run.Workload.State != "requested" {
			return xerrors.New("request not durable before create")
		}
		clock.Advance(provisionDuration)
		persisted <- struct{}{}
		return nil
	}
	s := testSupervisor(t, cfg, rt)
	s.clock = clock
	req := testRequest()
	req.Command = "printf '%s\\n' \"sensitive-command <>& '$HOME' `pwd`\"\nprintf 'second line\\n'\n"
	req.MemoryBytes = 96 << 20
	req.Env = map[string]string{"TOKEN": "private-env-value", "EMPTY": "", "MULTILINE": "first=1,2\nsecond\n"}
	// A canceled HTTP context must not cancel the admitted workload.
	data, err := json.Marshal(req)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r := httptest.NewRequest(http.MethodPost, "/workloads", bytes.NewReader(data)).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer "+cfg.Token)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), "sensitive-command")
	receive(t, persisted)
	receive(t, rt.waiting)
	receive(t, rt.outputObserved)
	live := getOutput(t, s, req.ID, "0")
	require.Equal(t, "running", live.Workload.State)
	require.Equal(t, new(provisionDuration.Nanoseconds()), live.Workload.Timings.ProvisionNS)
	require.Nil(t, live.Workload.Timings.ExecNS)
	require.Equal(t, int64(2), live.NextOffset)
	require.Equal(t, codersdk.WorkloadHostStats{Total: 1, Running: 1}, getStats(t, s))
	require.Equal(t, codersdk.WorkloadHostHistory{
		Items: []codersdk.WorkloadHistoryEntry{{Request: req, Workload: live.Workload}}, Total: 1, NextOffset: 1,
	}, getHistory(t, s, ""))

	s.mu.Lock()
	info := s.infoLocked()
	s.mu.Unlock()
	require.Equal(t, int64(1), info.AvailableCPU)
	require.Equal(t, cfg.MemoryBytes-req.MemoryBytes-runtimeOverhead, info.AvailableMemoryBytes)
	require.Equal(t, http.StatusOK, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
	changed := req
	changed.Command = "other-secret"
	conflict := requestHTTP(t, s, http.MethodPost, "/workloads", changed)
	require.Equal(t, http.StatusConflict, conflict.Code)
	require.NotContains(t, conflict.Body.String(), changed.Command)
	changed = req
	changed.Env = map[string]string{"TOKEN": "changed-env-secret"}
	conflict = requestHTTP(t, s, http.MethodPost, "/workloads", changed)
	require.Equal(t, http.StatusConflict, conflict.Code)
	require.NotContains(t, conflict.Body.String(), changed.Env["TOKEN"])
	require.NotContains(t, conflict.Body.String(), req.Env["TOKEN"])
	require.Equal(t, codersdk.WorkloadHostStats{Total: 1, Running: 1}, getStats(t, s))
	rt.finish <- 7
	joinRuns(t, s)
	out := getOutput(t, s, req.ID, "0")
	require.Equal(t, "failed", out.Workload.State)
	require.Equal(t, codersdk.WorkloadTimings{
		ProvisionNS: new(provisionDuration.Nanoseconds()),
		ExecNS:      new(executionDuration.Nanoseconds()),
	}, out.Workload.Timings)
	require.Equal(t, 7, *out.Workload.ExitCode)
	require.False(t, out.Workload.CleanupPending)
	require.Equal(t, []codersdk.WorkloadEvent{{Stream: "stdout", Data: []byte("hello")}, {Stream: "stderr", Data: []byte("error")}}, out.Events)
	require.Equal(t, int64(2), out.NextOffset)
	require.Equal(t, out.Events[1:], getOutput(t, s, req.ID, "1").Events)
	require.Empty(t, getOutput(t, s, req.ID, "2").Events)
	require.Equal(t, http.StatusBadRequest, requestHTTP(t, s, http.MethodGet, "/workloads/"+req.ID.String()+"?offset=3", nil).Code)
	s.mu.Lock()
	info = s.infoLocked()
	s.mu.Unlock()
	require.Equal(t, cfg.CPU, info.AvailableCPU)
	require.Equal(t, cfg.MemoryBytes, info.AvailableMemoryBytes)
	for _, name := range []string{"state.json", outputName(req.ID)} {
		stat, err := os.Stat(filepath.Join(cfg.StateDir, name))
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), stat.Mode().Perm())
	}
	require.Equal(t, http.StatusNoContent, requestHTTP(t, s, http.MethodPost, "/drain", nil).Code)
	stats := getStats(t, s)
	require.Equal(t, codersdk.WorkloadHostStats{Total: 1, Failed: 1}, stats)
	history := getHistory(t, s, "")
	require.Equal(t, codersdk.WorkloadHostHistory{
		Items: []codersdk.WorkloadHistoryEntry{{Request: req, Workload: out.Workload}}, Total: 1, NextOffset: 1,
	}, history)
	require.NoError(t, s.Close())
	recoveredRuntime := newFake()
	recovered := testSupervisor(t, cfg, recoveredRuntime)
	var recoveredInfo codersdk.WorkloadHostInfo
	require.NoError(t, json.Unmarshal(requestHTTP(t, recovered, http.MethodGet, "/info", nil).Body.Bytes(), &recoveredInfo))
	require.False(t, recoveredInfo.Draining)
	require.Equal(t, out, getOutput(t, recovered, req.ID, "0"))
	require.Equal(t, http.StatusOK, requestHTTP(t, recovered, http.MethodPost, "/workloads", req).Code)
	require.Equal(t, stats, getStats(t, recovered))
	require.Equal(t, history, getHistory(t, recovered, ""))
	require.Equal(t, http.StatusConflict, requestHTTP(t, recovered, http.MethodPost, "/workloads", changed).Code)
	for _, path := range []string{"/info", "/stats", "/workloads/" + req.ID.String()} {
		response := requestHTTP(t, recovered, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, response.Code)
		require.NotContains(t, response.Body.String(), req.Env["TOKEN"])
		require.NotContains(t, response.Body.String(), `"env"`)
	}
	creates, starts, removes := recoveredRuntime.counts()
	require.Zero(t, creates)
	require.Zero(t, starts)
	require.Zero(t, removes)
}

func TestExecutionDurationExcludesOutputDrain(t *testing.T) {
	t.Parallel()
	clock := quartz.NewMock(t)
	rt := newFake()
	rt.executionDuration = 123 * time.Nanosecond
	inspected := make(chan struct{})
	rt.onExecutionDuration = func(context.Context) { close(inspected) }
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	rt.attachment = reader
	s := testSupervisor(t, testConfig(t), rt)
	s.clock = clock
	req := testRequest()
	require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
	receive(t, rt.waiting)
	clock.Advance(time.Minute)
	rt.finish <- 0
	// Inspection must happen before the output stream closes or cleanup starts.
	receive(t, inspected)
	_, _, removes := rt.counts()
	require.Zero(t, removes)
	clock.Advance(time.Hour)
	require.NoError(t, writer.Close())
	joinRuns(t, s)
	out := getOutput(t, s, req.ID, "0")
	require.Equal(t, "succeeded", out.Workload.State)
	require.Equal(t, new(int64(0)), out.Workload.Timings.ProvisionNS)
	require.Equal(t, new(rt.executionDuration.Nanoseconds()), out.Workload.Timings.ExecNS)
}

func TestExecutionDurationFailureIgnored(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		exitCode int
		err      error
		state    string
	}{
		{name: "Success", state: "succeeded", err: xerrors.New("inspect secret")},
		{name: "Failure", state: "failed", exitCode: 7, err: context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rt := newFake()
			rt.executionDurationErr = tc.err
			var inspectionCtx context.Context
			rt.onExecutionDuration = func(ctx context.Context) { inspectionCtx = ctx }
			rt.finish <- tc.exitCode
			cfg := testConfig(t)
			s := testSupervisor(t, cfg, rt)
			req := testRequest()
			require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
			joinRuns(t, s)
			require.NotNil(t, inspectionCtx)
			deadline, ok := inspectionCtx.Deadline()
			require.True(t, ok, "timing lookup must be bounded")
			require.LessOrEqual(t, time.Until(deadline), operationTimeout)
			out := getOutput(t, s, req.ID, "0")
			require.Equal(t, tc.state, out.Workload.State)
			require.Equal(t, new(tc.exitCode), out.Workload.ExitCode)
			require.Empty(t, out.Workload.Failure)
			require.False(t, out.Workload.CleanupPending)
			require.Nil(t, out.Workload.Timings.ExecNS)
			require.Equal(t, http.StatusOK, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
			creates, starts, removes := rt.counts()
			require.Equal(t, 1, creates)
			require.Equal(t, 1, starts)
			require.Equal(t, 1, removes)
			require.NoError(t, s.Close())
			recovered := testSupervisor(t, cfg, newFake())
			require.Equal(t, out, getOutput(t, recovered, req.ID, "0"))
		})
	}
}

func TestConcurrentAdmission(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t)
	cfg.CPU = 1
	s := testSupervisor(t, cfg, newFake())
	var wg sync.WaitGroup
	statuses := make(chan int, 20)
	for range 20 {
		wg.Go(func() { statuses <- requestHTTP(t, s, http.MethodPost, "/workloads", testRequest()).Code })
	}
	wg.Wait()
	close(statuses)
	accepted := 0
	for status := range statuses {
		if status == http.StatusCreated {
			accepted++
		} else {
			require.Equal(t, http.StatusTooManyRequests, status)
		}
	}
	require.Equal(t, 1, accepted)
	s.mu.Lock()
	info := s.infoLocked()
	s.mu.Unlock()
	require.Zero(t, info.AvailableCPU)
}

func TestMemoryOverheadAndDrain(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t)
	cfg.MemoryBytes = 128 << 20
	s := testSupervisor(t, cfg, newFake())
	req := testRequest()
	require.Equal(t, http.StatusTooManyRequests, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
	req.MemoryBytes = 64 << 20
	require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
	require.Equal(t, http.StatusNoContent, requestHTTP(t, s, http.MethodPost, "/drain", nil).Code)
	require.Equal(t, http.StatusOK, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
	require.Equal(t, http.StatusServiceUnavailable, requestHTTP(t, s, http.MethodPost, "/workloads", testRequest()).Code)
	var info codersdk.WorkloadHostInfo
	require.NoError(t, json.Unmarshal(requestHTTP(t, s, http.MethodGet, "/info", nil).Body.Bytes(), &info))
	require.True(t, info.Draining)
	require.Zero(t, info.AvailableMemoryBytes)
}

func TestUnknownOutcomes(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"create", "start", "wait", "remove"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			cfg := testConfig(t)
			rt := newFake()
			secretErr := xerrors.New("sensitive-command daemon secret")
			switch phase {
			case "create":
				rt.createErr = secretErr
			case "start":
				rt.startErr = secretErr
			case "wait":
				rt.waitErr = secretErr
			case "remove":
				rt.removeErr = secretErr
				rt.finish <- 0
			}
			s := testSupervisor(t, cfg, rt)
			req := testRequest()
			require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
			joinRuns(t, s)
			out := getOutput(t, s, req.ID, "0")
			require.True(t, out.Workload.CleanupPending)
			if phase == "remove" {
				require.Equal(t, "succeeded", out.Workload.State)
				require.Equal(t, codersdk.WorkloadHostStats{Total: 1, Succeeded: 1}, getStats(t, s))
			} else {
				require.Equal(t, "unknown", out.Workload.State)
				require.Nil(t, out.Workload.Timings.ExecNS)
				require.Equal(t, codersdk.WorkloadHostStats{Total: 1, Unknown: 1}, getStats(t, s))
			}
			require.NotContains(t, out.Workload.Failure, "secret")
			require.NotContains(t, out.Workload.Failure, req.Command)
			require.Equal(t, http.StatusOK, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
			require.Equal(t, http.StatusServiceUnavailable, requestHTTP(t, s, http.MethodPost, "/workloads", testRequest()).Code)
			require.Equal(t, http.StatusConflict, requestHTTP(t, s, http.MethodDelete, "/workloads/"+req.ID.String(), nil).Code)
			s.mu.Lock()
			info := s.infoLocked()
			s.mu.Unlock()
			require.Equal(t, cfg.MemoryBytes-req.MemoryBytes-runtimeOverhead, info.AvailableMemoryBytes)
			creates, starts, _ := rt.counts()
			require.Equal(t, 1, creates)
			require.LessOrEqual(t, starts, 1)
			require.NoError(t, s.Close())
			_, err := newSupervisor(t.Context(), cfg, newFake())
			require.ErrorContains(t, err, "automatic replay is disabled")
			require.NotContains(t, err.Error(), req.Command)
		})
	}
}

func TestClosePreservesRunningWorkload(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t)
	rt := newFake()
	s := testSupervisor(t, cfg, rt)
	req := testRequest()
	require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
	receive(t, rt.waiting)
	require.NoError(t, s.Close())
	creates, starts, removes := rt.counts()
	require.Equal(t, 1, creates)
	require.Equal(t, 1, starts)
	require.Zero(t, removes)
	_, err := newSupervisor(t.Context(), cfg, newFake())
	require.ErrorContains(t, err, "outstanding workload state")
}

func TestCancellation(t *testing.T) {
	t.Parallel()
	for _, beforeStart := range []bool{false, true} {
		t.Run(map[bool]string{true: "DuringCreate", false: "Running"}[beforeStart], func(t *testing.T) {
			t.Parallel()
			rt := newFake()
			gate := make(chan struct{})
			if beforeStart {
				rt.createGate = gate
			}
			s := testSupervisor(t, testConfig(t), rt)
			req := testRequest()
			require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
			if beforeStart {
				receive(t, rt.created)
			} else {
				receive(t, rt.waiting)
			}
			require.Equal(t, http.StatusNoContent, requestHTTP(t, s, http.MethodDelete, "/workloads/"+req.ID.String(), nil).Code)
			close(gate)
			joinRuns(t, s)
			out := getOutput(t, s, req.ID, "0")
			require.Equal(t, "canceled", out.Workload.State)
			require.Nil(t, out.Workload.Timings.ExecNS)
			require.Equal(t, codersdk.WorkloadHostStats{Total: 1, Canceled: 1}, getStats(t, s))
			require.False(t, out.Workload.CleanupPending)
			_, starts, removes := rt.counts()
			require.Equal(t, 1, removes)
			if beforeStart {
				require.Zero(t, starts)
			} else {
				require.Equal(t, 1, starts)
			}
			require.Equal(t, http.StatusNoContent, requestHTTP(t, s, http.MethodDelete, "/workloads/"+req.ID.String(), nil).Code)
		})
	}
}

func TestAuthentication(t *testing.T) {
	t.Parallel()
	s := testSupervisor(t, testConfig(t), newFake())
	for _, route := range []string{"/info", "/stats", "/history", "/workloads", "/workloads/" + uuid.NewString(), "/drain", "/unknown"} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			for _, auth := range []string{"", "Bearer wrong", "Basic " + s.config.Token, "bearer " + s.config.Token} {
				r := httptest.NewRequest(method, route, nil)
				r.Header.Set("Authorization", auth)
				w := httptest.NewRecorder()
				s.ServeHTTP(w, r)
				require.Equal(t, http.StatusUnauthorized, w.Code)
				require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
				require.NotContains(t, w.Body.String(), s.config.Token)
			}
		}
	}
}

func TestHistoryAndPrivateState(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t)
	s := testSupervisor(t, cfg, newFake())
	_, err := newSupervisor(t.Context(), cfg, newFake())
	require.ErrorContains(t, err, "already running")
	// Synthetic completed entries exercise the history bound without launching
	// 1024 containers. They are deliberately never persisted or recovered.
	s.mu.Lock()
	for range maxHistory {
		id := uuid.New()
		s.state.Runs[id] = &record{}
	}
	s.mu.Unlock()
	require.Equal(t, http.StatusServiceUnavailable, requestHTTP(t, s, http.MethodPost, "/workloads", testRequest()).Code)
	require.NoError(t, s.Close())
	require.NoError(t, os.WriteFile(filepath.Join(cfg.StateDir, "state.json"), []byte("corrupt secret"), 0o600))
	_, err = newSupervisor(t.Context(), cfg, newFake())
	require.ErrorContains(t, err, "read durable workload state")
	require.NotContains(t, err.Error(), "secret")
	require.NoError(t, os.WriteFile(filepath.Join(cfg.StateDir, "state.json"), []byte("{}"), 0o600))
	_, err = newSupervisor(t.Context(), cfg, newFake())
	require.ErrorContains(t, err, "invalid durable workload state")
}

func TestPersistenceFailureDoesNotLaunch(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t)
	rt := newFake()
	s := testSupervisor(t, cfg, rt)
	restore := blockStatePersistence(t, cfg)
	req := testRequest()
	w := requestHTTP(t, s, http.MethodPost, "/workloads", req)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.NotContains(t, w.Body.String(), req.Command)
	joinRuns(t, s)
	out := assertPersistenceFailure(t, s, req)
	require.Nil(t, out.Workload.StartedAt)
	assertPersistenceFailureRecovery(t, s, req, restore)
	creates, starts, removes := rt.counts()
	require.Zero(t, creates)
	require.Zero(t, starts)
	require.Zero(t, removes)
}

func TestPersistenceFailureAfterRuntimeStart(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t)
	rt := newFake()
	gate := make(chan struct{})
	rt.createGate = gate
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	rt.attachment = reader
	s := testSupervisor(t, cfg, rt)
	req := testRequest()
	require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
	receive(t, rt.created)
	// Admission is durable, but the running state cannot be persisted.
	restore := blockStatePersistence(t, cfg)
	close(gate)
	receive(t, rt.started)
	joinRuns(t, s)
	out := assertPersistenceFailure(t, s, req)
	require.NotNil(t, out.Workload.StartedAt)
	require.True(t, out.Truncated)
	// Returning from execution must close and join a blocked output reader.
	_, err := writer.Write([]byte("trailing output"))
	require.ErrorIs(t, err, io.ErrClosedPipe)
	assertPersistenceFailureRecovery(t, s, req, restore)
	creates, starts, removes := rt.counts()
	require.Equal(t, 1, creates)
	require.Equal(t, 1, starts)
	require.Zero(t, removes)
}

func blockStatePersistence(t *testing.T, cfg Config) func() {
	t.Helper()
	path := filepath.Join(cfg.StateDir, "state.json")
	backup := filepath.Join(cfg.StateDir, "state.backup.json")
	require.NoError(t, os.Rename(path, backup))
	require.NoError(t, os.Mkdir(path, 0o700))
	return func() {
		require.NoError(t, os.Remove(path))
		require.NoError(t, os.Rename(backup, path))
	}
}

func assertPersistenceFailure(t *testing.T, s *Supervisor, req codersdk.StartHostWorkloadRequest) codersdk.WorkloadOutput {
	t.Helper()
	out := getOutput(t, s, req.ID, "0")
	require.Equal(t, "unknown", out.Workload.State)
	require.True(t, out.Workload.CleanupPending)
	require.Contains(t, out.Workload.Failure, "operator reconciliation")
	require.NotContains(t, out.Workload.Failure, req.Command)
	require.Nil(t, out.Workload.ExitCode)
	require.Nil(t, out.Workload.FinishedAt)
	w := requestHTTP(t, s, http.MethodPost, "/workloads", req)
	require.Equal(t, http.StatusOK, w.Code)
	var replay codersdk.Workload
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &replay))
	require.Equal(t, out.Workload, replay)
	require.Equal(t, http.StatusServiceUnavailable, requestHTTP(t, s, http.MethodPost, "/workloads", testRequest()).Code)
	for _, path := range []string{"/stats", "/history"} {
		unavailable := requestHTTP(t, s, http.MethodGet, path, nil)
		require.Equal(t, http.StatusServiceUnavailable, unavailable.Code)
		require.NotContains(t, unavailable.Body.String(), req.Command)
	}
	require.Equal(t, http.StatusConflict, requestHTTP(t, s, http.MethodDelete, "/workloads/"+req.ID.String(), nil).Code)
	var info codersdk.WorkloadHostInfo
	require.NoError(t, json.Unmarshal(requestHTTP(t, s, http.MethodGet, "/info", nil).Body.Bytes(), &info))
	require.True(t, info.Draining)
	require.Equal(t, s.config.CPU-req.CPU, info.AvailableCPU)
	require.Equal(t, s.config.MemoryBytes-req.MemoryBytes-runtimeOverhead, info.AvailableMemoryBytes)
	return out
}

func assertPersistenceFailureRecovery(t *testing.T, s *Supervisor, req codersdk.StartHostWorkloadRequest, restore func()) {
	t.Helper()
	// A later successful write must retain the unknown outcome and reservation,
	// not reenable admission or replay the command.
	restore()
	require.Equal(t, http.StatusNoContent, requestHTTP(t, s, http.MethodPost, "/drain", nil).Code)
	assertPersistenceFailure(t, s, req)
	var state durableState
	require.NoError(t, s.store.read("state.json", 1<<30, &state))
	require.Equal(t, "unknown", state.Runs[req.ID].Workload.State)
	require.True(t, state.Runs[req.ID].Reserved)
	require.True(t, state.Runs[req.ID].Workload.CleanupPending)
	require.NoError(t, s.Close())
	require.NoError(t, s.Close())
	_, err := newSupervisor(t.Context(), s.config, newFake())
	require.ErrorContains(t, err, "automatic replay is disabled")
}
