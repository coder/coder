//go:build linux

package agentnative

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/quartz"

	"github.com/coder/coder/v2/testutil"
)

type openCodeRestoreFixture struct {
	Session  Session          `json:"session"`
	Messages []map[string]any `json:"messages"`
}

// Seed a manager journal independently of the native server's lifecycle. The
// second manager uses the real OpenCode backend and process ownership checks.
type openCodeRestoreSeed struct {
	Backend
	host    Host
	session Session
	events  []Event
	state   json.RawMessage
}

func (b *openCodeRestoreSeed) Create(context.Context, string) (Session, error) { return b.session, nil }
func (b *openCodeRestoreSeed) Resume(_ context.Context, s Session) error {
	return b.host.Publish(s.ID, b.events, b.state)
}
func (*openCodeRestoreSeed) SetMode(context.Context, Session, string) error { return nil }
func (*openCodeRestoreSeed) Close() error                                   { return nil }

func TestOpenCodeColdRestoreProcessHelper(t *testing.T) {
	t.Parallel()
	if os.Getenv("CODER_OPENCODE_COLD_RESTORE_HELPER") != "1" {
		return
	}
	data, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), "restore-fixture.json"))
	require.NoError(t, err)
	var fixture openCodeRestoreFixture
	require.NoError(t, json.Unmarshal(data, &fixture))
	log, err := os.OpenFile(filepath.Join(os.Getenv("HOME"), "restore-requests.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	var logMu sync.Mutex
	port := os.Getenv("CODER_OPENCODE_COLD_RESTORE_PORT")
	listener, err := net.Listen("tcp4", "127.0.0.1:"+port)
	require.NoError(t, err)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logMu.Lock()
		_, _ = fmt.Fprintf(log, "%s %s\n", r.Method, r.URL.RequestURI())
		logMu.Unlock()
		_, password, _ := r.BasicAuth()
		if password != os.Getenv("OPENCODE_PASSWORD") {
			w.WriteHeader(401)
			return
		}
		if r.Method != "GET" {
			w.WriteHeader(500)
			return
		}
		if r.URL.Path == "/api/event" {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: {\"id\":\"ready\",\"type\":\"server.connected\",\"data\":{}}\n\n")
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		root := "/api/session/" + fixture.Session.NativeID
		var result any
		switch r.URL.Path {
		case "/api/info":
			result = map[string]any{"version": openCodeVersion, "pid": os.Getpid(), "urls": []string{"http://127.0.0.1:" + port}}
		case root:
			result = map[string]any{"data": map[string]any{"id": fixture.Session.NativeID, "location": map[string]string{"directory": fixture.Session.Cwd}, "time": map[string]int64{"created": fixture.Session.StartedAt}}}
		case root + "/message":
			result = map[string]any{"data": fixture.Messages, "cursor": map[string]any{}}
		case root + "/inbox", root + "/permission", root + "/form":
			result = map[string]any{"data": []any{}}
		case "/api/session/active":
			result = map[string]any{"data": map[string]any{}}
		case "/api/shell":
			result = map[string]any{"data": []any{}, "location": map[string]string{"directory": fixture.Session.Cwd}}
		default:
			w.WriteHeader(404)
			return
		}
		_ = json.NewEncoder(w).Encode(result)
	})
	ready := os.NewFile(3, "ready")
	_, _ = ready.Write([]byte{1})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: testutil.WaitLong}
	_ = server.Serve(listener)
}

func TestOpenCodeManagerColdRestoreKeepsIdentityWithoutReplay(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"absent", "dead", "old-domain"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			defer cancel()
			clock := quartz.NewMock(t)
			directory := filepath.Join(t.TempDir(), "state")
			nativeID := openCodeTestID("ses_")
			session := Session{ID: "opencode:" + nativeID, NativeID: nativeID, Harness: "opencode", Cwd: t.TempDir(), Mode: "human", StartedAt: 1234}
			idleID := openCodeTestID("msg_")
			receipts, err := readOpenCodeReceipts(nil)
			require.NoError(t, err)
			receipts.LastIdle = idleID
			state, err := json.Marshal(receipts)
			require.NoError(t, err)
			seed := &openCodeRestoreSeed{session: session, state: state, events: []Event{{Type: "text", Data: map[string]any{"text": "already observed"}}}}
			first, err := New(Options{StateDir: directory, Clock: clock, Factories: map[string]func(BackendOptions) (Backend, error){"opencode": func(options BackendOptions) (Backend, error) { seed.host = options.Host; return seed, nil }}})
			require.NoError(t, err)
			t.Cleanup(func() { _ = first.Close() })
			saved, err := first.Create(ctx, "opencode", session.Cwd)
			require.NoError(t, err)
			require.NoError(t, first.Close())

			home := filepath.Join(directory, "opencode-home")
			require.NoError(t, openCodeMakeHome(home))
			fixture := openCodeRestoreFixture{Session: session, Messages: []map[string]any{{"id": idleID, "type": "idle", "outcome": "succeeded"}}}
			data, err := json.Marshal(fixture)
			require.NoError(t, err)
			fixturePath := filepath.Join(home, "restore-fixture.json")
			require.NoError(t, os.WriteFile(fixturePath, data, 0o600))

			retired, err := net.Listen("tcp4", "127.0.0.1:0")
			require.NoError(t, err)
			defer retired.Close()
			domain, err := openCodeProcessDomain()
			require.NoError(t, err)
			old := openCodeDescriptor{Format: 1, Version: openCodeVersion, URL: "http://" + retired.Addr().String(), Password: strings.Repeat("a", 64), Marker: strings.Repeat("b", 64), Domain: domain, Status: "ready", PID: 1 << 30, StartTime: "1"}
			if kind == "old-domain" {
				old.Domain = "previous-boot/pid:[old]"
			}
			if kind != "absent" {
				server := newOpenCodeServer(BackendOptions{StateDir: directory}, clock)
				require.NoError(t, server.writeDescriptor(old))
			}

			reader, writer, err := os.Pipe()
			require.NoError(t, err)
			defer reader.Close()
			defer writer.Close()
			ready := make(chan struct{})
			go func() { var marker [1]byte; _, _ = io.ReadFull(reader, marker[:]); close(ready) }()
			var child *exec.Cmd
			var backend *OpenCode
			var parentWriter sync.Once
			starts := 0
			t.Cleanup(func() {
				if child != nil && child.Process != nil {
					_ = child.Process.Kill()
				}
			})
			second, err := New(Options{StateDir: directory, Clock: clock, Environment: []string{"PATH=/bin", "HOME=/unused"}, Factories: map[string]func(BackendOptions) (Backend, error){"opencode": func(options BackendOptions) (Backend, error) {
				options.Command = func(_ context.Context, path string, args ...string) *exec.Cmd {
					require.Equal(t, "/test/opencode", path)
					require.Equal(t, []string{"serve", "--hostname", "127.0.0.1", "--port"}, args[:4])
					starts++
					//nolint:gosec // The fixed script uses only this test binary and port.
					child = exec.Command("/bin/sh", "-c", `export CODER_OPENCODE_COLD_RESTORE_HELPER=1 CODER_OPENCODE_COLD_RESTORE_PORT="$1"; exec "$2" -test.run=^TestOpenCodeColdRestoreProcessHelper$`, "helper", args[4], os.Args[0])
					child.ExtraFiles = []*os.File{writer}
					return child
				}
				var err error
				backend, err = NewOpenCode(options)
				if err != nil {
					return nil, err
				}
				backend.server.version = func(context.Context) (string, error) { return "/test/opencode", nil }
				backend.server.process = func(pid int) (*openCodeProcess, error) {
					if kind == "old-domain" && pid == old.PID {
						t.Error("restore must not inspect a foreign-domain PID")
						return nil, errOpenCodeOwnership
					}
					if child != nil && child.Process != nil && pid == child.Process.Pid {
						parentWriter.Do(func() { _ = writer.Close() })
						select {
						case <-ready:
						case <-ctx.Done():
							return nil, ctx.Err()
						}
					}
					return openCodeProcessIdentity(pid)
				}
				backend.server.signal = func(openCodeDescriptor) error { return xerrors.New("restore must not signal any process") }
				return backend, nil
			}}})
			require.NoError(t, err)
			defer second.Close()
			require.NoError(t, retired.Close())
			require.NoError(t, second.Restore(ctx))
			require.NoError(t, second.Restore(ctx))
			require.Equal(t, 1, starts)
			current, err := second.Session(session.ID)
			require.NoError(t, err)
			require.Equal(t, saved.ID, current.ID)
			require.Equal(t, saved.NativeID, current.NativeID)
			require.Equal(t, saved.Cwd, current.Cwd)
			require.False(t, current.Archived)
			require.False(t, current.ResumePending)
			require.Empty(t, current.Fault)
			events, _, err := second.Events(ctx, session.ID, saved.Cursor, false)
			require.NoError(t, err)
			for _, event := range events {
				require.NotEqual(t, "text", event.Type)
				require.NotEqual(t, "turn.start", event.Type)
			}
			descriptor, err := backend.server.readDescriptor()
			require.NoError(t, err)
			require.NotEqual(t, old.Password, descriptor.Password)
			require.NotEqual(t, old.Marker, descriptor.Marker)
			if kind != "absent" {
				require.NotEqual(t, old.URL, descriptor.URL)
			}
			require.Equal(t, domain, descriptor.Domain)
			require.Contains(t, child.Env, "HOME="+home)
			require.NoError(t, second.Close())
			_, err = backend.server.connect(ctx)
			require.NoError(t, err, "closing the observer must leave the new server alive")
			requests, err := os.ReadFile(filepath.Join(home, "restore-requests.log"))
			require.NoError(t, err)
			for _, line := range strings.Split(strings.TrimSpace(string(requests)), "\n") {
				require.True(t, strings.HasPrefix(line, "GET "), line)
				require.NotContains(t, line, "/prompt")
			}
			preserved, err := os.ReadFile(fixturePath)
			require.NoError(t, err)
			require.Equal(t, data, preserved)
		})
	}
}

func TestOpenCodeColdRestoreRefusesOccupiedRetiredEndpoint(t *testing.T) {
	t.Parallel()
	for _, foreignDomain := range []bool{false, true} {
		t.Run(fmt.Sprint(foreignDomain), func(t *testing.T) {
			t.Parallel()
			f := newOpenCodeTestNative(t)
			o := f.backend(t)
			before, err := os.ReadFile(o.server.descriptorPath())
			require.NoError(t, err)
			if foreignDomain {
				o.server.domain = func() (string, error) { return "new-boot/new-pid-namespace", nil }
				o.server.process = func(int) (*openCodeProcess, error) {
					t.Error("a foreign-domain PID must never be inspected")
					return nil, errOpenCodeOwnership
				}
			} else {
				o.server.process = func(int) (*openCodeProcess, error) {
					return nil, nil //nolint:nilnil // Model confirmed death of the descriptor's process.
				}
			}
			o.server.version = func(context.Context) (string, error) {
				t.Error("occupied endpoint must refuse startup before CLI execution")
				return "", xerrors.New("unexpected CLI")
			}
			_, err = o.server.ensure(t.Context())
			require.Error(t, err)
			after, err := os.ReadFile(o.server.descriptorPath())
			require.NoError(t, err)
			require.Equal(t, before, after)
			f.mu.Lock()
			defer f.mu.Unlock()
			require.Empty(t, f.calls, "no saved credential may be sent to an unverified listener")
			require.Zero(t, f.signals)
		})
	}
}
