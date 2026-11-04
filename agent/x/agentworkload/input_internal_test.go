//go:build linux

package agentworkload

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type interactiveRuntime struct {
	*fakeRuntime
	resizes chan [2]uint16
}

func (f *interactiveRuntime) AttachInput(ctx context.Context, name string) (io.ReadCloser, error) {
	return f.Attach(ctx, name)
}

func (f *interactiveRuntime) Resize(ctx context.Context, _ string, width, height uint16) error {
	select {
	case f.resizes <- [2]uint16{width, height}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func attachmentFromConn(conn net.Conn) dockerAttachment {
	return dockerAttachment{client.ContainerAttachResult{HijackedResponse: client.NewHijackedResponse(conn, "")}}
}

func tcpAttachment(t *testing.T) (dockerAttachment, net.Conn) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	conn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	peer, err := listener.Accept()
	require.NoError(t, err)
	t.Cleanup(func() { _ = peer.Close() })
	return attachmentFromConn(conn), peer
}

func openInput(t *testing.T, server *httptest.Server, s *Supervisor, id uuid.UUID) *websocket.Conn {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
	conn, _, err := websocket.Dial(ctx, server.URL+"/workloads/"+id.String()+"/input", &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + s.config.Token}},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}

func TestInputEchoEOFAndDetach(t *testing.T) {
	t.Parallel()
	for _, tty := range []bool{false, true} {
		t.Run(map[bool]string{false: "Pipe", true: "TTY"}[tty], func(t *testing.T) {
			t.Parallel()
			attachment, peer := tcpAttachment(t)
			rt := &interactiveRuntime{fakeRuntime: newFake(), resizes: make(chan [2]uint16, 1)}
			rt.attachment = attachment
			s := testSupervisor(t, testConfig(t), rt)
			req := testRequest()
			req.Stdin, req.TTY = true, tty
			if tty {
				req.Width, req.Height = 80, 24
			}
			require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
			receive(t, rt.waiting)
			server, detached := streamServer(t, s)
			_, reader := openOutputStream(t, server, s, req.ID, 0)
			require.Empty(t, readOutputBatch(t, reader).Events)
			conn := openInput(t, server, s, req.ID)
			require.Equal(t, http.StatusConflict, requestHTTP(t, s, http.MethodGet, "/workloads/"+req.ID.String()+"/input", nil).Code)
			ctx := testutil.Context(t, testutil.WaitLong)
			if tty {
				require.NoError(t, wsjson.Write(ctx, conn, codersdk.WorkloadInput{Type: "resize", Width: 132, Height: 43}))
				require.Equal(t, [2]uint16{132, 43}, testutil.RequireReceive(ctx, t, rt.resizes))
			}
			echoDone := make(chan struct{})
			go func() {
				defer close(echoDone)
				defer peer.Close()
				defer func() { rt.finish <- 0 }()
				var buffer [16 << 10]byte
				for {
					n, err := peer.Read(buffer[:])
					if n > 0 {
						data := buffer[:n]
						if tty && bytes.Contains(data, []byte{4}) {
							return
						}
						if !tty {
							data = frame(1, data)
						}
						if _, err := peer.Write(data); err != nil {
							return
						}
					}
					if err != nil {
						return
					}
				}
			}()
			for _, text := range []string{"short prompt> ", "after detach\n"} {
				require.NoError(t, wsjson.Write(ctx, conn, codersdk.WorkloadInput{Type: "data", Data: []byte(text)}))
				batch := readOutputBatch(t, reader)
				require.Equal(t, []codersdk.WorkloadEvent{{Stream: "stdout", Data: []byte(text)}}, batch.Events)
				require.False(t, batch.Truncated)
				require.NoError(t, conn.CloseNow())
				receive(t, detached)
				s.mu.Lock()
				run := s.state.Runs[req.ID]
				closed := run.inputClosed
				s.mu.Unlock()
				require.False(t, closed)
				require.False(t, s.canceled(run))
				_, _, removes := rt.counts()
				require.Zero(t, removes)
				conn = openInput(t, server, s, req.ID)
			}
			require.NoError(t, wsjson.Write(ctx, conn, codersdk.WorkloadInput{Type: "eof"}))
			receive(t, echoDone)
			joinRuns(t, s)
			out := getOutput(t, s, req.ID, "0")
			require.Equal(t, "succeeded", out.Workload.State)
			require.Len(t, out.Events, 2)
			require.Equal(t, http.StatusConflict, requestHTTP(t, s, http.MethodGet, "/workloads/"+req.ID.String()+"/input", nil).Code)
			creates, starts, removes := rt.counts()
			require.Equal(t, 1, creates)
			require.Equal(t, 1, starts)
			require.Equal(t, 1, removes)
		})
	}
}

func TestInputValidation(t *testing.T) {
	t.Parallel()
	s := testSupervisor(t, testConfig(t), newFake())
	req := testRequest()
	require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
	for _, tc := range []struct {
		name, token string
		id          uuid.UUID
		status      int
	}{
		{name: "MissingToken", id: req.ID, status: http.StatusUnauthorized},
		{name: "WrongToken", token: "wrong", id: req.ID, status: http.StatusUnauthorized},
		{name: "NoStdin", token: s.config.Token, id: req.ID, status: http.StatusConflict},
		{name: "MissingRun", token: s.config.Token, id: uuid.New(), status: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(http.MethodGet, "/workloads/"+tc.id.String()+"/input", nil)
			r.Header.Set("Authorization", "Bearer "+tc.token)
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			require.Equal(t, tc.status, w.Code)
		})
	}
}

func TestInputWaitsForPrimaryAttachment(t *testing.T) {
	t.Parallel()
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "Ready", true: "Terminal"}[fail], func(t *testing.T) {
			t.Parallel()
			rt := newFake()
			gate := make(chan struct{})
			rt.createGate = gate
			attachment, _ := tcpAttachment(t)
			rt.attachment = attachment
			if fail {
				rt.createErr = io.ErrUnexpectedEOF
			}
			s := testSupervisor(t, testConfig(t), rt)
			req := testRequest()
			req.Stdin = true
			require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
			receive(t, rt.created)
			run, _, err := s.claimInput(req.ID)
			require.NoError(t, err)
			defer s.wg.Done()
			_, status, err := s.claimInput(req.ID)
			require.Error(t, err)
			require.Equal(t, http.StatusConflict, status)
			done := make(chan struct{})
			var input inputAttachment
			var inputErr error
			go func() {
				defer close(done)
				input, inputErr = s.waitInput(run.inputCtx, run)
			}()
			close(gate)
			receive(t, done)
			if fail {
				require.Error(t, inputErr)
				require.Nil(t, input)
			} else {
				require.NoError(t, inputErr)
				require.NotNil(t, input)
			}
		})
	}
}

type observedWriteConn struct {
	net.Conn
	writing chan struct{}
}

func (c *observedWriteConn) Write(data []byte) (int, error) {
	c.writing <- struct{}{}
	return c.Conn.Write(data)
}

func TestInputBlockedWriteCanceled(t *testing.T) {
	t.Parallel()
	for _, reason := range []string{"exit", "supervisor-close", "disconnect"} {
		t.Run(reason, func(t *testing.T) {
			t.Parallel()
			local, peer := net.Pipe()
			defer peer.Close()
			writing := make(chan struct{})
			rt := newFake()
			rt.attachment = attachmentFromConn(&observedWriteConn{Conn: local, writing: writing})
			s := testSupervisor(t, testConfig(t), rt)
			req := testRequest()
			req.Stdin = true
			require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
			receive(t, rt.waiting)
			server, detached := streamServer(t, s)
			conn := openInput(t, server, s, req.ID)
			ctx := testutil.Context(t, testutil.WaitLong)
			require.NoError(t, wsjson.Write(ctx, conn, codersdk.WorkloadInput{Type: "data", Data: []byte("blocked")}))
			receive(t, writing)
			switch reason {
			case "exit":
				rt.finish <- 0
			case "supervisor-close":
				require.NoError(t, s.Close())
			case "disconnect":
				require.NoError(t, conn.CloseNow())
			}
			receive(t, detached)
			if reason == "disconnect" {
				// Detachment must not close either side of the primary attach.
				_, err := peer.Write(frame(1, []byte("still running")))
				require.NoError(t, err)
				next := openInput(t, server, s, req.ID)
				require.NoError(t, wsjson.Write(ctx, next, codersdk.WorkloadInput{Type: "data", Data: []byte("next")}))
				receive(t, writing)
				var data [4]byte
				_, err = io.ReadFull(peer, data[:])
				require.NoError(t, err, "the canceled write deadline must not poison the next controller")
				require.Equal(t, "next", string(data[:]))
				require.NoError(t, s.Close())
			}
			require.NoError(t, peer.Close())
			joinRuns(t, s)
		})
	}
}
