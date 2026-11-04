package coderd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkloadInputProxy(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	req := f.request()
	req.Stdin, req.TTY, req.Width, req.Height = true, true, 80, 24
	f.inputHook = func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v0/workloads/workloads/"+req.ID.String()+"/input", r.URL.Path)
		assert.Empty(t, r.URL.RawQuery)
		assert.Equal(t, "Bearer "+f.token, r.Header.Get("Authorization"))
		for _, header := range []string{codersdk.SessionTokenHeader, "Cookie", "Test-Actor", "X-Private-Header"} {
			assert.Empty(t, r.Header.Get(header), header)
		}
		conn, err := websocket.Accept(w, r, nil)
		if !assert.NoError(t, err) {
			return
		}
		defer conn.CloseNow()
		conn.SetReadLimit(32 << 10)
		for range 3 {
			var input codersdk.WorkloadInput
			if !assert.NoError(t, wsjson.Read(ctx, conn, &input)) {
				return
			}
			assert.NoError(t, wsjson.Write(ctx, conn, input))
		}
		_ = conn.Close(websocket.StatusNormalClosure, "private supervisor close reason")
	}
	f.register(t)
	require.Equal(t, http.StatusCreated, f.call(t, "owner", http.MethodPost, "/workloads", req).Code)
	conn := dialWorkloadInputTest(t, f, ctx, req.ID)
	defer conn.CloseNow()
	for _, input := range []codersdk.WorkloadInput{
		{Type: "data", Data: bytes.Repeat([]byte("x"), 16<<10)},
		{Type: "resize", Width: 120, Height: 40},
		{Type: "eof"},
	} {
		require.NoError(t, wsjson.Write(ctx, conn, input))
		var got codersdk.WorkloadInput
		require.NoError(t, wsjson.Read(ctx, conn, &got))
		require.Equal(t, input, got)
	}
	_, _, err := conn.Read(ctx)
	require.Equal(t, websocket.StatusNormalClosure, websocket.CloseStatus(err))
	require.NotContains(t, err.Error(), "private supervisor close reason")
	waitWorkloadInputTest(t, f, ctx)
	// net/http pools upgrade connections separately from ordinary requests.
	require.EqualValues(t, 2, f.connections.Load(), "input must acquire only one additional tailnet connection")
	require.EqualValues(t, 4, f.requests.Load(), "exactly one input GET, without polling")
	require.EqualValues(t, 1, f.starts.Load())
}

func TestWorkloadInputClientCloseStatus(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	closed := make(chan error, 1)
	f.inputHook = func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if !assert.NoError(t, err) {
			return
		}
		defer conn.CloseNow()
		_, _, err = conn.Read(ctx)
		closed <- err
	}
	value := createWorkloadInputTest(t, f)
	conn := dialWorkloadInputTest(t, f, ctx, value.ID)
	defer conn.CloseNow()
	require.NoError(t, conn.Close(websocket.StatusNormalClosure, "private client close reason"))
	err := testutil.TryReceive(ctx, t, closed)
	require.Equal(t, websocket.StatusNormalClosure, websocket.CloseStatus(err))
	require.NotContains(t, err.Error(), "private client close reason")
	waitWorkloadInputTest(t, f, ctx)
	require.EqualValues(t, 4, f.requests.Load(), "a normal disconnect does not issue DELETE")
}

func TestWorkloadInputConcurrentDelete(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	deleted := make(chan struct{})
	f.outputHook = func(_ http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodDelete {
			close(deleted)
		}
		return false
	}
	f.inputHook = func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if !assert.NoError(t, err) {
			return
		}
		defer conn.CloseNow()
		select {
		case <-deleted:
			_ = conn.Close(websocket.StatusNormalClosure, "")
		case <-ctx.Done():
		}
	}
	value := createWorkloadInputTest(t, f)
	conn := dialWorkloadInputTest(t, f, ctx, value.ID)
	defer conn.CloseNow()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- f.call(t, "owner", http.MethodDelete, "/workloads/"+value.ID.String(), nil) }()
	require.Equal(t, http.StatusNoContent, testutil.TryReceive(ctx, t, done).Code)
	_, _, err := conn.Read(ctx)
	require.Equal(t, websocket.StatusNormalClosure, websocket.CloseStatus(err))
	waitWorkloadInputTest(t, f, ctx)
}

func TestWorkloadInputCancellation(t *testing.T) {
	t.Parallel()
	for _, shutdown := range []bool{false, true} {
		name := "Disconnect"
		if shutdown {
			name = "APIShutdown"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newWorkloadTest(t)
			ctx := testutil.Context(t, testutil.WaitLong)
			apiCtx, stopAPI := context.WithCancel(t.Context())
			defer stopAPI()
			f.api.ctx = apiCtx
			closed := make(chan struct{})
			var deletes atomic.Int32
			f.outputHook = func(_ http.ResponseWriter, r *http.Request) bool {
				if r.Method == http.MethodDelete {
					deletes.Add(1)
				}
				return false
			}
			f.inputHook = func(w http.ResponseWriter, r *http.Request) {
				defer close(closed)
				conn, err := websocket.Accept(w, r, nil)
				if !assert.NoError(t, err) {
					return
				}
				defer conn.CloseNow()
				_, _, _ = conn.Read(ctx)
			}
			value := createWorkloadInputTest(t, f)
			conn := dialWorkloadInputTest(t, f, ctx, value.ID)
			defer conn.CloseNow()
			if shutdown {
				stopAPI()
				_, _, err := conn.Read(ctx)
				require.Error(t, err)
			} else {
				require.NoError(t, conn.CloseNow())
			}
			testutil.TryReceive(ctx, t, closed)
			waitWorkloadInputTest(t, f, ctx)
			require.Zero(t, deletes.Load())
			require.EqualValues(t, 1, f.starts.Load())
			require.EqualValues(t, 4, f.requests.Load())
		})
	}
}

func TestWorkloadInputAuthorizationAndAdmission(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	value := createWorkloadInputTest(t, f)
	path := "/workloads/" + value.ID.String() + "/input"
	requests := f.requests.Load()
	for _, actor := range []string{"other", "template-admin", "foreign-admin", "owner-restricted"} {
		require.Equal(t, http.StatusNotFound, f.call(t, actor, http.MethodGet, path, nil).Code, actor)
	}
	require.Equal(t, http.StatusNotFound, f.callPath(t, "admin", http.MethodGet, "/organizations/"+uuid.NewString()+path, nil).Code)
	f.api.workloads.mu.Lock()
	f.api.workloads.runs[value.ID].value.State = "succeeded"
	f.api.workloads.mu.Unlock()
	for _, actor := range []string{"owner", "admin"} {
		require.Equal(t, http.StatusConflict, f.call(t, actor, http.MethodGet, path, nil).Code)
	}
	blocked := func(w http.ResponseWriter) bool { w.WriteHeader(http.StatusForbidden); return true }
	f.api.WorkspaceClientCoordinateOverride.Store(&blocked)
	require.Equal(t, http.StatusForbidden, f.call(t, "owner", http.MethodGet, path, nil).Code)
	f.api.WorkspaceClientCoordinateOverride.Store(nil)
	require.Equal(t, requests, f.requests.Load(), "denied input requests never contact a supervisor")

	noninteractive := f.request()
	require.Equal(t, http.StatusCreated, f.call(t, "owner", http.MethodPost, "/workloads", noninteractive).Code)
	require.Equal(t, http.StatusBadRequest, f.call(t, "owner", http.MethodGet, "/workloads/"+noninteractive.ID.String()+"/input", nil).Code)
	f.api.workloads.mu.Lock()
	run := f.api.workloads.runs[value.ID]
	run.host, run.value.State = nil, "running"
	f.api.workloads.mu.Unlock()
	require.Equal(t, http.StatusConflict, f.call(t, "owner", http.MethodGet, path, nil).Code)
}

func TestWorkloadInputHostErrors(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusBadRequest, http.StatusConflict, http.StatusForbidden, http.StatusServiceUnavailable, http.StatusTemporaryRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			f := newWorkloadTest(t)
			f.inputHook = func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", "http://example.invalid/secret")
				http.Error(w, "private-supervisor-error "+f.token, status)
			}
			value := createWorkloadInputTest(t, f)
			// No downstream upgrade occurs if the supervisor rejects input.
			res := f.call(t, "owner", http.MethodGet, "/workloads/"+value.ID.String()+"/input", nil)
			want := http.StatusBadGateway
			if status == http.StatusBadRequest || status == http.StatusConflict {
				want = status
			}
			require.Equal(t, want, res.Code)
			require.NotContains(t, res.Body.String(), "private-supervisor-error")
			require.NotContains(t, res.Body.String(), f.token)
			require.EqualValues(t, 4, f.requests.Load())
		})
	}
}

func TestWorkloadInputPendingDispatch(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	started, proceed := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(proceed) })
	defer unblock()
	f.startHook = func(_ http.ResponseWriter, r *http.Request) bool {
		close(started)
		select {
		case <-proceed:
		case <-r.Context().Done():
		}
		return false
	}
	f.inputHook = func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v0/workloads/workloads/"), "/input"))
		assert.NoError(t, err)
		f.api.workloads.mu.Lock()
		pending := f.api.workloads.runs[id].pending
		f.api.workloads.mu.Unlock()
		assert.False(t, pending, "input must wait for dispatch")
		http.Error(w, "not accepting input", http.StatusConflict)
	}
	f.register(t)
	req := f.request()
	req.Stdin = true
	created := make(chan *httptest.ResponseRecorder, 1)
	go func() { created <- f.call(t, "owner", http.MethodPost, "/workloads", req) }()
	testutil.TryReceive(ctx, t, started)
	path := "/organizations/" + f.orgID.String() + "/workloads/" + req.ID.String() + "/input"
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	done := make(chan struct{})
	go func() {
		r := httptest.NewRequestWithContext(canceled, http.MethodGet, path, nil)
		r.Header.Set("Test-Actor", "owner")
		f.router.ServeHTTP(httptest.NewRecorder(), r)
		close(done)
	}()
	testutil.TryReceive(ctx, t, done)
	input := make(chan *httptest.ResponseRecorder, 1)
	go func() { input <- f.callPath(t, "owner", http.MethodGet, path, nil) }()
	unblock()
	require.Equal(t, http.StatusCreated, testutil.TryReceive(ctx, t, created).Code)
	require.Equal(t, http.StatusConflict, testutil.TryReceive(ctx, t, input).Code)
	require.EqualValues(t, 4, f.requests.Load())
}

func TestWorkloadInputMessageLimit(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	closed := make(chan struct{})
	f.inputHook = func(w http.ResponseWriter, r *http.Request) {
		defer close(closed)
		conn, err := websocket.Accept(w, r, nil)
		if !assert.NoError(t, err) {
			return
		}
		defer conn.CloseNow()
		conn.SetReadLimit(64 << 10)
		_, data, err := conn.Read(ctx)
		assert.Error(t, err)
		assert.Empty(t, data, "oversized messages must not reach the supervisor")
	}
	value := createWorkloadInputTest(t, f)
	conn := dialWorkloadInputTest(t, f, ctx, value.ID)
	defer conn.CloseNow()
	_ = conn.Write(ctx, websocket.MessageText, bytes.Repeat([]byte("x"), (32<<10)+1))
	_, _, err := conn.Read(ctx)
	require.Equal(t, websocket.StatusMessageTooBig, websocket.CloseStatus(err))
	testutil.TryReceive(ctx, t, closed)
	waitWorkloadInputTest(t, f, ctx)
}

func createWorkloadInputTest(t *testing.T, f *workloadTest) codersdk.Workload {
	t.Helper()
	f.register(t)
	req := f.request()
	req.Stdin = true
	res := f.call(t, "owner", http.MethodPost, "/workloads", req)
	require.Equal(t, http.StatusCreated, res.Code, res.Body.String())
	var value codersdk.Workload
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &value))
	require.Equal(t, "running", value.State)
	return value
}

func dialWorkloadInputTest(t *testing.T, f *workloadTest, ctx context.Context, id uuid.UUID) *websocket.Conn {
	t.Helper()
	server := httptest.NewServer(compressHandler(f.router))
	t.Cleanup(server.Close)
	conn, res, err := websocket.Dial(ctx, server.URL+"/organizations/"+f.orgID.String()+"/workloads/"+id.String()+"/input", &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Test-Actor": {"owner"}, "Authorization": {"Bearer user-token"},
			codersdk.SessionTokenHeader: {"must-not-reach-supervisor"},
			"Cookie":                    {"private=cookie"}, "X-Private-Header": {"secret"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, res.StatusCode)
	conn.SetReadLimit(32 << 10)
	return conn
}

func waitWorkloadInputTest(t *testing.T, f *workloadTest, ctx context.Context) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		f.api.WebsocketWaitGroup.Wait()
		close(done)
	}()
	testutil.TryReceive(ctx, t, done)
}
