package coderd

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkloadFollowStreams(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	finish := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(finish) })
	defer unblock()
	f.outputHook = func(w http.ResponseWriter, r *http.Request) bool {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "7", r.URL.Query().Get("offset"))
		assert.Equal(t, "true", r.URL.Query().Get("follow"))
		assert.Equal(t, "application/x-ndjson", r.Header.Get("Accept"))
		assert.Empty(t, r.Header.Get(codersdk.SessionTokenHeader))
		value := workloadFollowValue(f, r)
		// Only supervisor-owned observation fields can reach the caller.
		value.AgentID = uuid.New()
		value.CreatedAt = time.Time{}
		value.Generation.Workdir = f.token
		value.Generation.Repositories = map[string]string{"secret": f.token}
		value.Timings.HostConnectNS = new(int64(-100))
		value.Timings.HostConnectionReused = new(false)
		value.Timings.ProvisionNS = new(int64(10))
		value.Failure = f.token
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, "\n")
		assert.NoError(t, json.NewEncoder(w).Encode(codersdk.WorkloadOutput{
			Workload: value, NextOffset: 8, Events: []codersdk.WorkloadEvent{{Stream: "stdout", Data: []byte("first")}},
		}))
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
			return true
		case <-finish:
		}
		value.State = "succeeded"
		// A terminal frame with events does not mean output has drained.
		assert.NoError(t, json.NewEncoder(w).Encode(codersdk.WorkloadOutput{
			Workload: value, NextOffset: 9, Events: []codersdk.WorkloadEvent{{Stream: "stderr", Data: []byte("last")}},
		}))
		assert.NoError(t, json.NewEncoder(w).Encode(codersdk.WorkloadOutput{Workload: value, NextOffset: 9}))
		return true
	}
	value := createWorkloadFollowTest(t, f)
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
	defer cancel()
	res := requestWorkloadFollow(t, f, ctx, value.ID, 7)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, "application/x-ndjson", res.Header.Get("Content-Type"))
	scanner := bufio.NewScanner(res.Body)
	require.True(t, scanner.Scan(), scanner.Err())
	require.Empty(t, scanner.Text(), "heartbeats must be forwarded immediately")
	first := readWorkloadFollowFrame(t, scanner)
	require.Equal(t, "running", first.Workload.State)
	require.Equal(t, []byte("first"), first.Events[0].Data)
	require.EqualValues(t, 8, first.NextOffset)
	require.Equal(t, value.AgentID, first.Workload.AgentID)
	require.Equal(t, value.CreatedAt, first.Workload.CreatedAt)
	require.Equal(t, value.Generation, first.Workload.Generation)
	require.Equal(t, value.Timings.HostConnectNS, first.Workload.Timings.HostConnectNS)
	require.Equal(t, value.Timings.HostConnectionReused, first.Workload.Timings.HostConnectionReused)
	require.Equal(t, new(int64(10)), first.Workload.Timings.ProvisionNS)
	require.Equal(t, "Workload supervisor reported an execution or cleanup failure.", first.Workload.Failure)
	require.NotContains(t, scanner.Text(), f.token)
	require.EqualValues(t, 1, f.connections.Load(), "follow must reuse the pooled tailnet connection")
	unblock()
	last := readWorkloadFollowFrame(t, scanner)
	require.Equal(t, "succeeded", last.Workload.State)
	require.Equal(t, []byte("last"), last.Events[0].Data)
	terminal := readWorkloadFollowFrame(t, scanner)
	require.True(t, terminal.Workload.Terminal())
	require.Empty(t, terminal.Events)
	require.EqualValues(t, 9, terminal.NextOffset)
	require.False(t, scanner.Scan())
	require.NoError(t, scanner.Err())
	f.api.workloads.mu.Lock()
	stored := f.api.workloads.runs[value.ID].value
	f.api.workloads.mu.Unlock()
	require.Equal(t, terminal.Workload.State, stored.State)
	require.Equal(t, terminal.Workload.Timings, stored.Timings)
	require.Equal(t, terminal.Workload.Failure, stored.Failure)
	require.EqualValues(t, 4, f.requests.Load(), "one follow GET, with no polling or replay")
	require.EqualValues(t, 1, f.starts.Load())
}

func TestWorkloadFollowTruncatedOffsetGap(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	f.outputHook = func(w http.ResponseWriter, r *http.Request) bool {
		value := workloadFollowValue(f, r)
		value.State = "succeeded"
		w.Header().Set("Content-Type", "application/x-ndjson")
		assert.NoError(t, json.NewEncoder(w).Encode(codersdk.WorkloadOutput{
			Workload: value, NextOffset: 100, Truncated: true,
			Events: []codersdk.WorkloadEvent{{Stream: "stdout", Data: []byte("retained")}},
		}))
		assert.NoError(t, json.NewEncoder(w).Encode(codersdk.WorkloadOutput{
			Workload: value, NextOffset: 100, Truncated: true,
		}))
		return true
	}
	value := createWorkloadFollowTest(t, f)
	res := f.call(t, "owner", http.MethodGet, "/workloads/"+value.ID.String()+"?follow=true&offset=7", nil)
	require.Equal(t, http.StatusOK, res.Code)
	scanner := bufio.NewScanner(res.Body)
	first := readWorkloadFollowFrame(t, scanner)
	require.True(t, first.Truncated)
	require.EqualValues(t, 100, first.NextOffset)
	require.Equal(t, []byte("retained"), first.Events[0].Data)
	require.Empty(t, readWorkloadFollowFrame(t, scanner).Events)
	require.False(t, scanner.Scan())
	require.NoError(t, scanner.Err())
}

func TestWorkloadFollowConcurrentDelete(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	deleted := make(chan struct{})
	f.outputHook = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodDelete {
			close(deleted)
			return false
		}
		value := workloadFollowValue(f, r)
		w.Header().Set("Content-Type", "application/x-ndjson")
		assert.NoError(t, json.NewEncoder(w).Encode(codersdk.WorkloadOutput{Workload: value}))
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
			return true
		case <-deleted:
		}
		value.State = "canceled"
		assert.NoError(t, json.NewEncoder(w).Encode(codersdk.WorkloadOutput{Workload: value}))
		return true
	}
	value := createWorkloadFollowTest(t, f)
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
	defer cancel()
	res := requestWorkloadFollow(t, f, ctx, value.ID, 0)
	defer res.Body.Close()
	scanner := bufio.NewScanner(res.Body)
	require.Equal(t, "running", readWorkloadFollowFrame(t, scanner).Workload.State)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- f.call(t, "owner", http.MethodDelete, "/workloads/"+value.ID.String(), nil) }()
	require.Equal(t, http.StatusNoContent, testutil.TryReceive(ctx, t, done).Code)
	require.Equal(t, "canceled", readWorkloadFollowFrame(t, scanner).Workload.State)
	require.False(t, scanner.Scan())
	require.NoError(t, scanner.Err())
}

func TestWorkloadFollowCancellation(t *testing.T) {
	t.Parallel()
	for _, shutdown := range []bool{false, true} {
		name := "ClientDisconnect"
		if shutdown {
			name = "APIShutdown"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newWorkloadTest(t)
			apiCtx, stopAPI := context.WithCancel(t.Context())
			defer stopAPI()
			f.api.ctx = apiCtx
			upstreamClosed := make(chan struct{})
			var deletes atomic.Int64
			f.outputHook = func(w http.ResponseWriter, r *http.Request) bool {
				if r.Method == http.MethodDelete {
					deletes.Add(1)
					return false
				}
				w.Header().Set("Content-Type", "application/x-ndjson")
				assert.NoError(t, json.NewEncoder(w).Encode(codersdk.WorkloadOutput{Workload: workloadFollowValue(f, r)}))
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				close(upstreamClosed)
				return true
			}
			value := createWorkloadFollowTest(t, f)
			ctx := testutil.Context(t, testutil.WaitLong)
			clientCtx, stopClient := context.WithCancel(ctx)
			defer stopClient()
			res := requestWorkloadFollow(t, f, clientCtx, value.ID, 0)
			defer res.Body.Close()
			scanner := bufio.NewScanner(res.Body)
			require.Equal(t, "running", readWorkloadFollowFrame(t, scanner).Workload.State)
			if shutdown {
				stopAPI()
			} else {
				stopClient()
			}
			testutil.TryReceive(ctx, t, upstreamClosed)
			require.False(t, scanner.Scan())
			require.Zero(t, deletes.Load(), "disconnecting a stream must not cancel execution")
			require.EqualValues(t, 1, f.starts.Load())
			require.EqualValues(t, 4, f.requests.Load())
		})
	}
}

func TestWorkloadFollowPendingDispatch(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	var registered atomic.Bool
	dispatching, proceed := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(proceed) })
	defer unblock()
	f.infoHook = func(_ http.ResponseWriter, r *http.Request) bool {
		if registered.Load() {
			close(dispatching)
			select {
			case <-r.Context().Done():
			case <-proceed:
			}
		}
		return false
	}
	f.outputHook = func(w http.ResponseWriter, r *http.Request) bool {
		value := workloadFollowValue(f, r)
		assert.Equal(t, "running", value.State, "observation must wait for dispatch completion")
		assert.NotNil(t, value.Timings.HostConnectNS)
		value.State = "succeeded"
		w.Header().Set("Content-Type", "application/x-ndjson")
		assert.NoError(t, json.NewEncoder(w).Encode(codersdk.WorkloadOutput{Workload: value}))
		return true
	}
	f.register(t)
	registered.Store(true)
	req := f.request()
	ctx := testutil.Context(t, testutil.WaitLong)
	created := make(chan *httptest.ResponseRecorder, 1)
	go func() { created <- f.call(t, "owner", http.MethodPost, "/workloads", req) }()
	testutil.TryReceive(ctx, t, dispatching)
	f.api.workloads.mu.Lock()
	run := f.api.workloads.runs[req.ID]
	host, pending := run.host, run.pending
	f.api.workloads.mu.Unlock()
	require.Nil(t, host)
	require.True(t, pending)

	// A canceled observer can leave while dispatch still owns run.op.
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	done := make(chan struct{})
	go func() {
		f.api.followWorkload(httptest.NewRecorder(), httptest.NewRequestWithContext(canceled, http.MethodGet, "/", nil), run, 0)
		close(done)
	}()
	testutil.TryReceive(ctx, t, done)

	observing := make(chan struct{})
	follow := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		close(observing)
		follow <- f.call(t, "owner", http.MethodGet, "/workloads/"+req.ID.String()+"?follow=true", nil)
	}()
	testutil.TryReceive(ctx, t, observing)
	unblock()
	require.Equal(t, http.StatusCreated, testutil.TryReceive(ctx, t, created).Code)
	res := testutil.TryReceive(ctx, t, follow)
	require.Equal(t, http.StatusOK, res.Code)
	var output codersdk.WorkloadOutput
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &output))
	require.Equal(t, "succeeded", output.Workload.State)
	require.Empty(t, output.Events)
}

func TestWorkloadFollowLocalAndAuthorization(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	req := f.request()
	res := f.call(t, "owner", http.MethodPost, "/workloads", req)
	var value codersdk.Workload
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &value))
	require.Equal(t, "rejected", value.State)
	path := "/workloads/" + req.ID.String() + "?follow=true&offset=7"
	for _, actor := range []string{"other", "template-admin", "foreign-admin", "owner-restricted"} {
		require.Equal(t, http.StatusNotFound, f.call(t, actor, http.MethodGet, path, nil).Code, actor)
	}
	foreignPath := "/organizations/" + uuid.NewString() + path
	require.Equal(t, http.StatusNotFound, f.callPath(t, "admin", http.MethodGet, foreignPath, nil).Code)
	for _, actor := range []string{"owner", "admin"} {
		res = f.call(t, actor, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, res.Code)
		require.Equal(t, "application/x-ndjson", res.Header().Get("Content-Type"))
		require.True(t, res.Flushed)
		var output codersdk.WorkloadOutput
		require.NoError(t, json.Unmarshal(res.Body.Bytes(), &output))
		require.Equal(t, value, output.Workload, "local failure details must remain unchanged")
		require.Empty(t, output.Events)
		require.EqualValues(t, 7, output.NextOffset)
		require.True(t, strings.HasSuffix(res.Body.String(), "\n"))
	}
	for _, offset := range []string{"-1", "invalid", "9223372036854775808"} {
		path := "/workloads/" + req.ID.String() + "?follow=true&offset=" + offset
		require.Equal(t, http.StatusBadRequest, f.call(t, "owner", http.MethodGet, path, nil).Code)
	}
	blocked := func(w http.ResponseWriter) bool { w.WriteHeader(http.StatusConflict); return true }
	f.api.WorkspaceClientCoordinateOverride.Store(&blocked)
	require.Equal(t, http.StatusConflict, f.call(t, "owner", http.MethodGet, path, nil).Code)
	require.Zero(t, f.requests.Load(), "denied and local requests never contact a supervisor")
}

func TestWorkloadFollowInvalidHostResponse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		status      int
		contentType string
		change      func(*codersdk.WorkloadOutput)
		body        string
	}{
		{name: "BadOffset", status: http.StatusBadRequest},
		{name: "HostError", status: http.StatusServiceUnavailable},
		{name: "Redirect", status: http.StatusTemporaryRedirect},
		{name: "InvalidContentType", contentType: "application/json"},
		{name: "InvalidJSON", body: "private-host-secret\n"},
		{name: "OversizedFrame", body: strings.Repeat("x", workloadStreamFrameLimit+1) + "\n"},
		{name: "OffsetGap", change: func(o *codersdk.WorkloadOutput) { o.NextOffset = 9 }},
		{name: "OffsetRollback", change: func(o *codersdk.WorkloadOutput) { o.NextOffset = 6 }},
		{name: "TruncatedOffsetRollback", change: func(o *codersdk.WorkloadOutput) { o.Truncated, o.NextOffset = true, 6 }},
		{name: "TruncatedEventCount", change: func(o *codersdk.WorkloadOutput) { o.Truncated, o.NextOffset = true, 7 }},
		{name: "EventCount", change: func(o *codersdk.WorkloadOutput) { o.Events = nil }},
		{name: "SpoofedOwner", change: func(o *codersdk.WorkloadOutput) { o.Workload.OwnerID = uuid.New() }},
		{name: "InvalidState", change: func(o *codersdk.WorkloadOutput) { o.Workload.State = "private-host-secret" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newWorkloadTest(t)
			f.outputHook = func(w http.ResponseWriter, r *http.Request) bool {
				w.Header().Set("Content-Type", "application/x-ndjson")
				if tc.contentType != "" {
					w.Header().Set("Content-Type", tc.contentType)
					_, _ = io.WriteString(w, "private-host-secret")
					return true
				}
				if tc.status != 0 {
					w.Header().Set("Location", "http://example.invalid/private-host-secret")
					w.WriteHeader(tc.status)
					_, _ = io.WriteString(w, "private-host-secret "+f.token)
					return true
				}
				if tc.body != "" {
					_, _ = io.WriteString(w, tc.body)
					return true
				}
				output := codersdk.WorkloadOutput{
					Workload: workloadFollowValue(f, r), NextOffset: 8,
					Events: []codersdk.WorkloadEvent{{Stream: "stdout", Data: []byte("private-host-secret")}},
				}
				tc.change(&output)
				assert.NoError(t, json.NewEncoder(w).Encode(output))
				return true
			}
			value := createWorkloadFollowTest(t, f)
			f.api.workloads.mu.Lock()
			before := f.api.workloads.runs[value.ID].value
			f.api.workloads.mu.Unlock()
			res := f.call(t, "owner", http.MethodGet, "/workloads/"+value.ID.String()+"?follow=true&offset=7", nil)
			switch {
			case tc.status == http.StatusBadRequest:
				require.Equal(t, http.StatusBadRequest, res.Code)
			case tc.status != 0 || tc.contentType != "":
				require.Equal(t, http.StatusBadGateway, res.Code)
			default:
				require.Equal(t, http.StatusOK, res.Code)
				require.Empty(t, res.Body.String(), "invalid frames must not appear drained or expose output")
			}
			require.NotContains(t, res.Body.String(), f.token)
			require.NotContains(t, res.Body.String(), "private-host-secret")
			f.api.workloads.mu.Lock()
			stored := f.api.workloads.runs[value.ID].value
			f.api.workloads.mu.Unlock()
			require.Equal(t, before, stored, "invalid frames cannot mutate authoritative observations")
		})
	}
}

func TestWorkloadFollowLargeTotalOutput(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	const frames = 40
	data := []byte(strings.Repeat("x", 128<<10))
	f.outputHook = func(w http.ResponseWriter, r *http.Request) bool {
		value := workloadFollowValue(f, r)
		w.Header().Set("Content-Type", "application/x-ndjson")
		for i := range frames {
			if !assert.NoError(t, json.NewEncoder(w).Encode(codersdk.WorkloadOutput{
				Workload: value, NextOffset: int64(i + 1), Events: []codersdk.WorkloadEvent{{Stream: "stdout", Data: data}},
			})) {
				return true
			}
			w.(http.Flusher).Flush()
		}
		value.State = "succeeded"
		assert.NoError(t, json.NewEncoder(w).Encode(codersdk.WorkloadOutput{Workload: value, NextOffset: frames}))
		return true
	}
	value := createWorkloadFollowTest(t, f)
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
	defer cancel()
	res := requestWorkloadFollow(t, f, ctx, value.ID, 0)
	defer res.Body.Close()
	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 64<<10), workloadStreamFrameLimit+1)
	for i := range frames {
		output := readWorkloadFollowFrame(t, scanner)
		require.EqualValues(t, i+1, output.NextOffset)
		require.Len(t, output.Events, 1)
		require.Equal(t, data, output.Events[0].Data)
	}
	require.Empty(t, readWorkloadFollowFrame(t, scanner).Events)
	require.False(t, scanner.Scan())
	require.NoError(t, scanner.Err())
	require.Greater(t, frames*len(data), workloadStreamFrameLimit)
}

func createWorkloadFollowTest(t *testing.T, f *workloadTest) codersdk.Workload {
	t.Helper()
	f.register(t)
	res := f.call(t, "owner", http.MethodPost, "/workloads", f.request())
	require.Equal(t, http.StatusCreated, res.Code, res.Body.String())
	var value codersdk.Workload
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &value))
	require.Equal(t, "running", value.State)
	return value
}

func workloadFollowValue(f *workloadTest, r *http.Request) codersdk.Workload {
	id, _ := uuid.Parse(strings.TrimPrefix(r.URL.Path, "/api/v0/workloads/workloads/"))
	f.api.workloads.mu.Lock()
	defer f.api.workloads.mu.Unlock()
	return f.api.workloads.runs[id].value
}

func requestWorkloadFollow(t *testing.T, f *workloadTest, ctx context.Context, id uuid.UUID, offset int64) *http.Response {
	t.Helper()
	server := httptest.NewServer(compressHandler(f.router))
	t.Cleanup(server.Close)
	path := "/organizations/" + f.orgID.String() + "/workloads/" + id.String() + "?follow=true&offset=" + strconv.FormatInt(offset, 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+path, nil)
	require.NoError(t, err)
	req.Header.Set("Test-Actor", "owner")
	req.Header.Set(codersdk.SessionTokenHeader, "must-not-reach-supervisor")
	res, err := server.Client().Do(req)
	require.NoError(t, err)
	return res
}

func readWorkloadFollowFrame(t *testing.T, scanner *bufio.Scanner) codersdk.WorkloadOutput {
	t.Helper()
	require.True(t, scanner.Scan(), scanner.Err())
	var output codersdk.WorkloadOutput
	require.NoError(t, json.Unmarshal(scanner.Bytes(), &output))
	return output
}
