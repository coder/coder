package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/serpent"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestRunInteractiveInput(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	org, id := uuid.New(), uuid.New()
	input := "first line\nsecond line\x00\x03"
	inputRead := make(chan []byte, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v2/organizations/{org}/workloads/{id}/input", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, org.String(), r.PathValue("org"))
		assert.Equal(t, id.String(), r.PathValue("id"))
		conn, err := websocket.Accept(w, r, nil)
		if !assert.NoError(t, err) {
			return
		}
		defer conn.CloseNow()
		var received bytes.Buffer
		for {
			var event codersdk.WorkloadInput
			if !assert.NoError(t, wsjson.Read(r.Context(), conn, &event)) {
				return
			}
			switch event.Type {
			case "data":
				received.Write(event.Data)
			case "eof":
				inputRead <- received.Bytes()
				_ = conn.Close(websocket.StatusNormalClosure, "")
				return
			default:
				t.Errorf("unexpected input type %q", event.Type)
				return
			}
		}
	})
	mux.HandleFunc("GET /api/v2/organizations/{org}/workloads/{id}", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "true", r.URL.Query().Get("follow"))
		writeRunTestStream(t, w, codersdk.WorkloadOutput{Workload: codersdk.Workload{State: "running"}})
		var received []byte
		select {
		case received = <-inputRead:
		case <-r.Context().Done():
			return
		}
		writeRunTestStream(t, w,
			codersdk.WorkloadOutput{Workload: codersdk.Workload{State: "failed", ExitCode: new(23)}, NextOffset: 2,
				Events: []codersdk.WorkloadEvent{{Stream: "stdout", Data: received}, {Stream: "stderr", Data: []byte("stderr")}}},
			codersdk.WorkloadOutput{Workload: codersdk.Workload{State: "failed", ExitCode: new(23)}, NextOffset: 2},
		)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	var stdout, stderr bytes.Buffer
	inv := &serpent.Invocation{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	err = watchInteractiveWorkload(ctx, inv, codersdk.New(serverURL), org, codersdk.CreateWorkloadRequest{ID: id, Stdin: true}, quartz.NewMock(t), nil)
	var exit *exitError
	require.ErrorAs(t, err, &exit)
	require.Equal(t, 23, exit.ExitCode())
	require.Equal(t, input, stdout.String())
	require.Equal(t, "stderr", stderr.String())
}

func TestRunInteractiveDisconnect(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	input, inputWriter, err := os.Pipe()
	require.NoError(t, err)
	defer input.Close()
	defer inputWriter.Close()
	outputOpened := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v2/organizations/{org}/workloads/{id}/input", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if !assert.NoError(t, err) {
			return
		}
		defer conn.CloseNow()
		select {
		case <-outputOpened:
		case <-r.Context().Done():
			return
		}
		_ = conn.Close(websocket.StatusInternalError, "input transport failed")
	})
	mux.HandleFunc("GET /api/v2/organizations/{org}/workloads/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeRunTestStream(t, w, codersdk.WorkloadOutput{Workload: codersdk.Workload{State: "running"}})
		close(outputOpened)
		<-r.Context().Done()
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("input disconnection must not cancel or relaunch: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	var stdout, stderr bytes.Buffer
	inv := &serpent.Invocation{Stdin: input, Stdout: &stdout, Stderr: &stderr}
	err = watchInteractiveWorkload(ctx, inv, codersdk.New(serverURL), uuid.New(), codersdk.CreateWorkloadRequest{ID: uuid.New(), Stdin: true}, quartz.NewMock(t), nil)
	require.ErrorContains(t, err, "input connection closed")
	require.ErrorContains(t, err, "not canceled")
	require.Empty(t, stdout.String())
	require.Empty(t, stderr.String())
}

func TestRunInteractiveFastExit(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		if strings.HasSuffix(r.URL.Path, "/input") {
			w.WriteHeader(http.StatusConflict)
			return
		}
		output := codersdk.WorkloadOutput{Workload: codersdk.Workload{State: "succeeded", ExitCode: new(0)}}
		if r.URL.Query().Get("follow") == "true" {
			writeRunTestStream(t, w, output)
		} else {
			writeRunTestJSON(t, w, output)
		}
	}))
	defer server.Close()
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	var stdout, stderr bytes.Buffer
	inv := &serpent.Invocation{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	err = watchInteractiveWorkload(ctx, inv, codersdk.New(serverURL), uuid.New(), codersdk.CreateWorkloadRequest{ID: uuid.New(), Stdin: true}, quartz.NewMock(t), nil)
	require.NoError(t, err)
}

func TestRunTTYRequiresTerminal(t *testing.T) {
	t.Parallel()
	inv, stdout, stderr := newRunTestInvocation(t, "http://127.0.0.1:1", quartz.NewMock(t), "--template", "prepared", "--cpu", "1", "--ram", "1GiB", "--cmd", "claude", "-it")
	require.ErrorContains(t, inv.Run(), "--tty requires a local")
	require.Empty(t, stdout.String())
	require.Empty(t, stderr.String())
}
