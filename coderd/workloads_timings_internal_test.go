package coderd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptrace"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkloadDispatchConnectionTimings(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		coldInfo    bool
		coldPost    bool
		wantReused  bool
		connections int64
	}{
		{name: "ColdInfoPooledPOST", coldInfo: true, connections: 2},
		{name: "PooledInfoPooledPOST", wantReused: true, connections: 1},
		{name: "PooledInfoColdPOST", coldPost: true, connections: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newWorkloadTest(t)
			var registered atomic.Bool
			f.infoHook = func(w http.ResponseWriter, _ *http.Request) bool {
				if registered.Load() && tc.coldPost {
					w.Header().Set("Connection", "close")
				}
				return false
			}
			f.register(t)
			registered.Store(true)
			// Registration warms the pool; a cold dispatch must close that socket.
			if tc.coldInfo {
				f.api.workloadTransport().CloseIdleConnections()
			}
			req := f.request()
			created := f.call(t, "owner", http.MethodPost, "/workloads", req)
			require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
			var value codersdk.Workload
			require.NoError(t, json.Unmarshal(created.Body.Bytes(), &value))
			require.Equal(t, "running", value.State)
			require.NotNil(t, value.Timings.HostConnectNS)
			require.GreaterOrEqual(t, *value.Timings.HostConnectNS, int64(0))
			require.Equal(t, new(tc.wantReused), value.Timings.HostConnectionReused)
			require.Nil(t, value.Timings.ProvisionNS)
			require.Nil(t, value.Timings.ExecNS)
			require.Equal(t, tc.connections, f.connections.Load())
			require.EqualValues(t, 3, f.requests.Load(), "registration, dispatch info, and dispatch POST")

			duplicate := f.call(t, "owner", http.MethodPost, "/workloads", req)
			require.Equal(t, http.StatusOK, duplicate.Code)
			require.JSONEq(t, created.Body.String(), duplicate.Body.String())
			require.EqualValues(t, 3, f.requests.Load(), "idempotent create must not acquire another connection")

			// Force a fresh polling connection so reuse differs from a pooled
			// dispatch. Polls must retain the original dispatch measurement.
			f.api.workloadTransport().CloseIdleConnections()
			output := f.call(t, "owner", http.MethodGet, "/workloads/"+req.ID.String(), nil)
			require.Equal(t, http.StatusOK, output.Code, output.Body.String())
			var batch codersdk.WorkloadOutput
			require.NoError(t, json.Unmarshal(output.Body.Bytes(), &batch))
			require.Equal(t, value.Timings, batch.Workload.Timings)
			require.Equal(t, tc.connections+1, f.connections.Load())
			duplicate = f.call(t, "owner", http.MethodPost, "/workloads", req)
			require.Equal(t, http.StatusOK, duplicate.Code)
			require.JSONEq(t, created.Body.String(), duplicate.Body.String())
			require.EqualValues(t, 1, f.starts.Load())
		})
	}
}

func TestWorkloadDispatchConnectionTimingsUnavailable(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	req := f.request()
	created := f.call(t, "owner", http.MethodPost, "/workloads", req)
	require.Equal(t, http.StatusCreated, created.Code)
	var value codersdk.Workload
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &value))
	require.Equal(t, "rejected", value.State)
	require.Equal(t, codersdk.WorkloadTimings{}, value.Timings)
	require.Zero(t, f.connections.Load())
	duplicate := f.call(t, "owner", http.MethodPost, "/workloads", req)
	require.Equal(t, http.StatusOK, duplicate.Code)
	require.JSONEq(t, created.Body.String(), duplicate.Body.String())
}

func TestWorkloadConnectionTiming(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		reused     []bool
		wantReused bool
	}{
		{name: "AllReused", reused: []bool{true, true}, wantReused: true},
		{name: "FirstCold", reused: []bool{false, true}},
		{name: "LastCold", reused: []bool{true, false}},
		{name: "AllCold", reused: []bool{false, false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var timing workloadConnectionTiming
			trace := timing.trace()
			ns, reused := timing.result()
			require.Nil(t, ns)
			require.Nil(t, reused)
			// Seed acquisition starts in the past without waiting on real time.
			// Bounds allow callback overhead but distinguish a sum from the
			// duration of only the last acquisition.
			started := time.Now()
			var expected int64
			for i, reused := range tc.reused {
				trace.GetConn("host")
				duration := time.Duration(i+1) * time.Second
				timing.mu.Lock()
				timing.started = timing.started.Add(-duration)
				timing.mu.Unlock()
				trace.GotConn(httptrace.GotConnInfo{Reused: reused})
				expected += duration.Nanoseconds()
			}
			ns, reused = timing.result()
			require.NotNil(t, ns)
			require.GreaterOrEqual(t, *ns, expected)
			require.LessOrEqual(t, *ns, expected+time.Since(started).Nanoseconds())
			require.Equal(t, new(tc.wantReused), reused)

			// No completed acquisition means no extra duration, even if a
			// later request fails while waiting for its connection.
			trace.GetConn("host")
			afterNS, afterReused := timing.result()
			require.Equal(t, ns, afterNS)
			require.Equal(t, reused, afterReused)
			// Results are snapshots, not pointers into the mutable trace.
			snapshotNS := *ns
			trace.GotConn(httptrace.GotConnInfo{Reused: false})
			require.Equal(t, new(tc.wantReused), reused)
			require.Equal(t, snapshotNS, *ns)
			require.Equal(t, snapshotNS, *afterNS)
		})
	}
	t.Run("AcquisitionFails", func(t *testing.T) {
		t.Parallel()
		var timing workloadConnectionTiming
		timing.trace().GetConn("host")
		ns, reused := timing.result()
		require.Nil(t, ns)
		require.Nil(t, reused)
	})
}

func TestWorkloadConnectionTimingExcludesResponseTime(t *testing.T) {
	t.Parallel()
	requested, headersSent := make(chan struct{}), make(chan struct{})
	allowHeaders, allowBody := make(chan struct{}), make(chan struct{})
	unblockHeaders := sync.OnceFunc(func() { close(allowHeaders) })
	unblockBody := sync.OnceFunc(func() { close(allowBody) })
	defer unblockHeaders()
	defer unblockBody()
	f := newWorkloadTransportTest(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(requested)
		<-allowHeaders
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(headersSent)
		<-allowBody
		_, _ = io.WriteString(w, "{}")
	}))
	var timing workloadConnectionTiming
	ctx := testutil.Context(t, testutil.WaitLong)
	ctx = httptrace.WithClientTrace(ctx, timing.trace())
	done := make(chan error, 1)
	go func() {
		_, err := f.api.workloadHostRequest(ctx, f.host, http.MethodPost, "/workloads", struct{}{}, nil)
		done <- err
	}()
	testutil.TryReceive(ctx, t, requested)
	connectNS, reused := timing.result()
	require.NotNil(t, connectNS, "connection measurement completes before host processing")
	require.Equal(t, new(false), reused)
	unblockHeaders()
	testutil.TryReceive(ctx, t, headersSent)
	afterHeaders, _ := timing.result()
	require.Equal(t, connectNS, afterHeaders)
	unblockBody()
	require.NoError(t, testutil.TryReceive(ctx, t, done))
	afterBody, _ := timing.result()
	require.Equal(t, connectNS, afterBody, "reading the response body must not extend acquisition time")
}

func TestWorkloadObservationTimings(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		provisionNS   *int64
		execNS        *int64
		wantProvision *int64
		wantExec      *int64
	}{
		{name: "Available", provisionNS: new(int64(12)), execNS: new(int64(34)), wantProvision: new(int64(12)), wantExec: new(int64(34))},
		{name: "Zero", provisionNS: new(int64(0)), execNS: new(int64(0)), wantProvision: new(int64(0)), wantExec: new(int64(0))},
		{name: "Unavailable"},
		{name: "Negative", provisionNS: new(int64(-1)), execNS: new(int64(-2))},
		{name: "ProvisionOnly", provisionNS: new(int64(12)), execNS: new(int64(-1)), wantProvision: new(int64(12))},
		{name: "ExecOnly", provisionNS: new(int64(-1)), execNS: new(int64(34)), wantExec: new(int64(34))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			value := codersdk.Workload{ID: uuid.New(), State: "running", Timings: codersdk.WorkloadTimings{
				HostConnectNS: new(int64(56)), HostConnectionReused: new(true),
				ProvisionNS: new(int64(78)), ExecNS: new(int64(90)),
			}}
			observed := value
			observed.State = "succeeded"
			observed.Timings = codersdk.WorkloadTimings{
				HostConnectNS: new(int64(999)), HostConnectionReused: new(false),
				ProvisionNS: tc.provisionNS, ExecNS: tc.execNS,
			}
			original := value
			observed.ID = uuid.New()
			require.False(t, mergeWorkloadObservation(&value, observed))
			require.Equal(t, original, value, "invalid observations must not change timings")
			observed.ID = value.ID
			require.True(t, mergeWorkloadObservation(&value, observed))
			require.Equal(t, "succeeded", value.State)
			require.Equal(t, codersdk.WorkloadTimings{
				HostConnectNS: original.Timings.HostConnectNS, HostConnectionReused: original.Timings.HostConnectionReused,
				ProvisionNS: tc.wantProvision, ExecNS: tc.wantExec,
			}, value.Timings)
			// A supervisor cannot manufacture an unavailable server measurement.
			value.Timings.HostConnectNS, value.Timings.HostConnectionReused = nil, nil
			require.True(t, mergeWorkloadObservation(&value, observed))
			require.Nil(t, value.Timings.HostConnectNS)
			require.Nil(t, value.Timings.HostConnectionReused)
		})
	}
}
