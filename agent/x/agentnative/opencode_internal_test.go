//go:build linux

package agentnative

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/quartz"

	"github.com/coder/coder/v2/testutil"
)

type openCodeTestHost struct {
	mu      sync.Mutex
	session Session
	events  []Event
	changed chan struct{}
	fail    bool
}

func (h *openCodeTestHost) Session(id string) (Session, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.session.ID != id {
		return Session{}, xerrors.New("unknown test session")
	}
	s := h.session
	s.State = slices.Clone(s.State)
	return s, nil
}

func (h *openCodeTestHost) Publish(id string, events []Event, state json.RawMessage) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.fail {
		return xerrors.New("private secret persistence diagnostic")
	}
	if h.session.ID != id {
		return xerrors.New("unknown test session")
	}
	data, _ := json.Marshal(events)
	var copied []Event
	_ = json.Unmarshal(data, &copied)
	h.events = append(h.events, copied...)
	if len(state) != 0 {
		h.session.State = slices.Clone(state)
	}
	for _, event := range copied {
		if event.Type == "mode" {
			h.session.Mode, _ = event.Data["mode"].(string)
		}
		if event.Type == "overflow" {
			h.session.Fault = "observation failed"
		}
	}
	close(h.changed)
	h.changed = make(chan struct{})
	return nil
}

func (h *openCodeTestHost) all() []Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.events)
}

func (h *openCodeTestHost) automation() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.session.Mode = "automation"
}

func (h *openCodeTestHost) wait(t *testing.T, predicate func([]Event) bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	defer cancel()
	for {
		h.mu.Lock()
		ok := predicate(h.events)
		changed := h.changed
		h.mu.Unlock()
		if ok {
			return
		}
		select {
		case <-changed:
		case <-ctx.Done():
			t.Fatal("timed out waiting for native observations")
		}
	}
}

type openCodeTestCall struct {
	method, path string
	body         map[string]any
}

type openCodeTestNative struct {
	mu             sync.Mutex
	server         *httptest.Server
	descriptor     openCodeDescriptor
	directory      string
	id             string
	host           *openCodeTestHost
	messages       []map[string]any
	inbox          []map[string]any
	calls          []openCodeTestCall
	streams        map[chan openCodeNativeEvent]bool
	promptStatus   int
	losePrompt     bool
	deleteDelivers bool
	active         bool
	more           bool
	oversize       bool
	signals        int
}

func openCodeTestID(prefix string) string {
	return prefix + strings.ReplaceAll(uuid.NewString(), "-", "")[:26]
}

func newOpenCodeTestNative(t *testing.T) *openCodeTestNative {
	t.Helper()
	f := &openCodeTestNative{directory: t.TempDir(), id: openCodeTestID("ses_"), host: &openCodeTestHost{changed: make(chan struct{})}, messages: []map[string]any{}, inbox: []map[string]any{}, streams: map[chan openCodeNativeEvent]bool{}}
	require.NoError(t, os.Chmod(f.directory, 0o700))
	f.descriptor = openCodeDescriptor{Format: 1, Version: openCodeVersion, Password: strings.Repeat("a", 64), Marker: strings.Repeat("b", 64), Domain: "test-domain", Status: "ready", PID: 12345, StartTime: "6789"}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	f.descriptor.URL = f.server.URL
	t.Cleanup(f.server.Close)
	return f
}

func (f *openCodeTestNative) backend(t *testing.T) *OpenCode {
	t.Helper()
	o, err := NewOpenCode(BackendOptions{StateDir: f.directory, Host: f.host, Clock: quartz.NewMock(t), Environment: []string{"HOME=/original", "PATH=/bin", "OPENCODE_DB=private-secret", "OPENCODE_PASSWORD=private-secret"}})
	require.NoError(t, err)
	o.server.version = func(context.Context) (string, error) { return "/test/opencode", nil }
	o.server.domain = func() (string, error) { return f.descriptor.Domain, nil }
	o.server.process = func(int) (*openCodeProcess, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.signals > 0 {
			return nil, nil //nolint:nilnil // A missing process is a successful liveness check.
		}
		_, portText, _ := net.SplitHostPort(f.server.Listener.Addr().String())
		port, _ := strconv.Atoi(portText)
		return &openCodeProcess{StartTime: f.descriptor.StartTime, Marker: f.descriptor.Marker, Ports: []int{port}}, nil
	}
	o.server.signal = func(openCodeDescriptor) error { f.mu.Lock(); defer f.mu.Unlock(); f.signals++; return nil }
	require.NoError(t, o.server.writeDescriptor(f.descriptor))
	t.Cleanup(func() { require.NoError(t, o.Close()) })
	return o
}

func (f *openCodeTestNative) create(t *testing.T, o *OpenCode) Session {
	t.Helper()
	s, err := o.Create(t.Context(), "/workspace")
	require.NoError(t, err)
	f.host.mu.Lock()
	f.host.session = s
	f.host.mu.Unlock()
	require.NoError(t, o.Resume(t.Context(), s))
	return s
}

func (f *openCodeTestNative) serve(w http.ResponseWriter, r *http.Request) {
	_, password, _ := r.BasicAuth()
	if password != f.descriptor.Password {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.URL.Path == "/api/event" {
		w.Header().Set("Content-Type", "text/event-stream")
		stream := make(chan openCodeNativeEvent, 128)
		f.mu.Lock()
		f.streams[stream] = true
		f.mu.Unlock()
		defer func() { f.mu.Lock(); delete(f.streams, stream); f.mu.Unlock() }()
		_, _ = fmt.Fprint(w, "data: {\"id\":\"connected\",\"type\":\"server.connected\",\"data\":{}}\n\n")
		w.(http.Flusher).Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case event := <-stream:
				data, _ := json.Marshal(event)
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				w.(http.Flusher).Flush()
			}
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var body map[string]any
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	f.calls = append(f.calls, openCodeTestCall{r.Method, r.URL.RequestURI(), body})
	w.Header().Set("Content-Type", "application/json")
	var result any
	root := "/api/session/" + f.id
	switch {
	case r.URL.Path == "/api/info":
		result = map[string]any{"version": openCodeVersion, "pid": f.descriptor.PID, "urls": []string{f.descriptor.URL}}
	case r.URL.Path == "/api/session" || r.URL.Path == root:
		result = map[string]any{"data": map[string]any{"id": f.id, "location": map[string]string{"directory": "/workspace"}, "time": map[string]int{"created": 1234}}}
	case r.URL.Path == root+"/message":
		if f.oversize {
			_, _ = w.Write([]byte(`{"data":"` + strings.Repeat("s", openCodeMaxResponse) + `"}`))
			return
		}
		data := slices.Clone(f.messages)
		if r.URL.Query().Get("cursor") != "" {
			if r.URL.Query().Get("order") != "" || r.URL.Query().Get("limit") != "1" {
				w.WriteHeader(400)
				return
			}
			data = []map[string]any{}
			if f.more {
				data = append(data, map[string]any{"id": openCodeTestID("msg_"), "type": "system"})
			}
		} else {
			if r.URL.Query().Get("limit") != "200" {
				w.WriteHeader(400)
				return
			}
			slices.Reverse(data)
		}
		cursor := map[string]any{"previous": nil, "next": nil}
		if len(data) != 0 {
			cursor["next"] = data[len(data)-1]["id"]
			cursor["previous"] = data[0]["id"]
		}
		result = map[string]any{"data": data, "cursor": cursor}
	case r.URL.Path == root+"/inbox":
		result = map[string]any{"data": f.inbox}
	case r.URL.Path == "/api/session/active":
		active := map[string]any{}
		if f.active {
			active[f.id] = map[string]string{"type": "running"}
		}
		result = map[string]any{"data": active}
	case r.URL.Path == root+"/permission" || r.URL.Path == root+"/form":
		result = map[string]any{"data": []any{}}
	case r.URL.Path == "/api/shell":
		result = map[string]any{"data": []any{}, "location": map[string]string{"directory": "/workspace"}}
	case r.URL.Path == root+"/prompt":
		if f.promptStatus != 0 {
			w.WriteHeader(f.promptStatus)
			_, _ = fmt.Fprint(w, "private-secret")
			return
		}
		item := map[string]any{"id": body["id"], "type": "user", "sessionID": f.id, "text": body["text"], "payload": map[string]any{"text": body["text"]}}
		f.inbox = append(f.inbox, item)
		if f.losePrompt {
			connection, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = connection.Close()
			}
			return
		}
		result = map[string]any{"data": item}
	case strings.HasPrefix(r.URL.Path, root+"/inbox/") && r.Method == "DELETE":
		id := strings.TrimPrefix(r.URL.Path, root+"/inbox/")
		for index, item := range f.inbox {
			if item["id"] != id {
				continue
			}
			f.inbox = slices.Delete(f.inbox, index, index+1)
			if f.deleteDelivers {
				f.messages = append(f.messages, map[string]any{"id": id, "type": "user", "text": item["text"]})
			} else {
				f.emitLocked("session.inbox.cancelled", map[string]any{"inboxID": id})
			}
			break
		}
		w.WriteHeader(204)
		return
	default:
		w.WriteHeader(404)
		return
	}
	_ = json.NewEncoder(w).Encode(result)
}

func (f *openCodeTestNative) emitLocked(kind string, data map[string]any) {
	data["sessionID"] = f.id
	raw := map[string]json.RawMessage{}
	for key, value := range data {
		raw[key], _ = json.Marshal(value)
	}
	for stream := range f.streams {
		stream <- openCodeNativeEvent{ID: uuid.NewString(), Type: kind, Data: raw}
	}
}

func (f *openCodeTestNative) mutate(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn()
	f.emitLocked("session.message.updated", map[string]any{})
}

func (f *openCodeTestNative) mutations() []openCodeTestCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []openCodeTestCall
	for _, call := range f.calls {
		if call.method != "GET" {
			result = append(result, call)
		}
	}
	return result
}

func openCodeTestText(text string) map[string]any {
	return map[string]any{"type": "text", "text": text}
}
func openCodeTestAssistant(parts ...map[string]any) map[string]any {
	return map[string]any{"id": openCodeTestID("msg_"), "type": "assistant", "content": parts}
}
func openCodeTestIdle() map[string]any {
	return map[string]any{"id": openCodeTestID("msg_"), "type": "idle", "outcome": "succeeded"}
}
func openCodeHasEvent(events []Event, kind string) bool {
	return slices.ContainsFunc(events, func(e Event) bool { return e.Type == kind })
}

func TestOpenCodePassiveConstruction(t *testing.T) {
	t.Parallel()
	directory := filepath.Join(t.TempDir(), "absent")
	o, err := NewOpenCode(BackendOptions{StateDir: directory, Host: &openCodeTestHost{}, Command: func(context.Context, string, ...string) *exec.Cmd {
		t.Fatal("constructor must not discover CLI")
		return nil
	}})
	require.NoError(t, err)
	require.NoError(t, o.Close())
	_, err = os.Stat(directory)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestOpenCodeHTTPAndSSEProjection(t *testing.T) {
	t.Parallel()
	f := newOpenCodeTestNative(t)
	o := f.backend(t)
	s := f.create(t, o)
	require.Equal(t, "opencode:"+f.id, s.ID)
	f.host.automation()
	prompt := Prompt{ID: uuid.NewString(), Text: "hello"}
	require.NoError(t, o.Prompt(t.Context(), s, prompt))
	calls := f.mutations()
	require.Len(t, calls, 2)
	nativeID := calls[1].body["id"].(string)
	require.Equal(t, "queue", calls[1].body["delivery"])
	tool := map[string]any{"type": "tool", "id": "tool1", "name": "shell", "state": map[string]any{"status": "streaming", "input": `{"command":`}}
	answer := openCodeTestText("answer")
	f.mutate(func() {
		f.inbox = []map[string]any{}
		f.messages = append(f.messages, map[string]any{"id": nativeID, "type": "user", "text": prompt.Text}, openCodeTestAssistant(map[string]any{"type": "reasoning", "text": "thinking"}, answer, tool))
	})
	f.host.wait(t, func(events []Event) bool { return openCodeHasEvent(events, "tool.start") })
	f.mutate(func() {
		answer["text"] = "answer more"
		tool["state"] = map[string]any{"status": "completed", "input": map[string]any{"command": "pwd"}, "content": []any{map[string]any{"type": "text", "text": "/workspace"}}}
		f.messages = append(f.messages, openCodeTestIdle())
	})
	f.host.wait(t, func(events []Event) bool { return openCodeHasEvent(events, "turn.complete") })
	var kinds []string
	for _, event := range f.host.all() {
		if event.Data["turnId"] != nativeID {
			continue
		}
		kinds = append(kinds, event.Type)
		require.Equal(t, prompt.ID, event.Data["requestId"])
		if event.Type == "tool.start" {
			require.Equal(t, `{"command":`, event.Data["input"])
		}
		if event.Type == "tool.end" {
			require.Equal(t, map[string]any{"command": "pwd"}, event.Data["input"])
			require.NotNil(t, event.Data["result"])
		}
	}
	require.Equal(t, []string{"turn.start", "thought", "text", "tool.start", "text", "tool.end", "turn.complete"}, kinds)
	require.NoError(t, o.Prompt(t.Context(), s, prompt))
	require.Len(t, f.mutations(), 2)
}

func TestOpenCodeRestartReceiptsNoReplay(t *testing.T) {
	t.Parallel()
	f := newOpenCodeTestNative(t)
	o := f.backend(t)
	s := f.create(t, o)
	f.host.automation()
	prompt := Prompt{ID: uuid.NewString(), Text: "hello"}
	require.NoError(t, o.Prompt(t.Context(), s, prompt))
	nativeID := f.mutations()[1].body["id"].(string)
	answer := openCodeTestText("before🌏")
	f.mutate(func() {
		f.inbox = []map[string]any{}
		f.messages = append(f.messages, map[string]any{"id": nativeID, "type": "user", "text": "hello"}, openCodeTestAssistant(answer))
	})
	f.host.wait(t, func(events []Event) bool { return openCodeHasEvent(events, "text") })
	require.NoError(t, o.Close())
	f.mu.Lock()
	answer["text"] = "before🌏 after"
	f.mu.Unlock()
	restored := f.backend(t)
	saved, err := f.host.Session(s.ID)
	require.NoError(t, err)
	require.NoError(t, restored.Resume(t.Context(), saved))
	var texts, starts []string
	for _, event := range f.host.all() {
		if event.Type == "text" {
			texts = append(texts, event.Data["text"].(string))
		}
		if event.Type == "turn.start" {
			starts = append(starts, event.Data["text"].(string))
			require.Equal(t, prompt.ID, event.Data["requestId"])
		}
	}
	require.Equal(t, []string{"before🌏", " after"}, texts)
	require.Equal(t, []string{"hello", ""}, starts)
	require.Len(t, f.mutations(), 2)
	require.NoError(t, restored.Prompt(t.Context(), saved, prompt))
	require.Len(t, f.mutations(), 2)
	f.mutate(func() { f.messages = append(f.messages, openCodeTestIdle()) })
	f.host.wait(t, func(events []Event) bool { return openCodeHasEvent(events, "turn.complete") })
}

func TestOpenCodeNativeSyntheticTurnAndReadOnlyFailure(t *testing.T) {
	t.Parallel()
	for _, change := range []string{"prefix", "missing", "anchor", "more", "oversize", "count"} {
		t.Run(change, func(t *testing.T) {
			t.Parallel()
			f := newOpenCodeTestNative(t)
			o := f.backend(t)
			s := f.create(t, o)
			f.host.automation()
			answer := openCodeTestText("original")
			f.mutate(func() {
				f.messages = append(f.messages, map[string]any{"id": openCodeTestID("msg_"), "type": "synthetic", "text": "native continuation"}, openCodeTestAssistant(answer))
			})
			f.host.wait(t, func(events []Event) bool { return openCodeHasEvent(events, "text") })
			for _, event := range f.host.all() {
				require.NotContains(t, event.Data, "requestId")
			}
			saved, err := f.host.Session(s.ID)
			require.NoError(t, err)
			require.Equal(t, "human", saved.Mode)
			if change == "anchor" {
				f.mutate(func() { f.messages = append(f.messages, openCodeTestIdle()) })
				f.host.wait(t, func(events []Event) bool { return openCodeHasEvent(events, "turn.complete") })
			}
			require.NoError(t, o.Close())
			f.mu.Lock()
			switch change {
			case "prefix":
				answer["text"] = "rewritten"
			case "missing", "anchor":
				f.messages = []map[string]any{}
			case "more":
				f.more = true
			case "oversize":
				f.oversize = true
			case "count":
				for len(f.messages) <= 200 {
					f.messages = append(f.messages, map[string]any{"id": openCodeTestID("msg_"), "type": "system"})
				}
			}
			f.mu.Unlock()
			restored := f.backend(t)
			err = restored.Resume(t.Context(), saved)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "private-secret")
			require.Len(t, f.mutations(), 1)
			var texts []string
			for _, event := range f.host.all() {
				if event.Type == "text" {
					texts = append(texts, event.Data["text"].(string))
				}
			}
			require.Equal(t, []string{"original"}, texts)
		})
	}
}

func TestOpenCodeExactCancellation(t *testing.T) {
	t.Parallel()
	for _, race := range []bool{false, true} {
		t.Run(fmt.Sprint(race), func(t *testing.T) {
			t.Parallel()
			f := newOpenCodeTestNative(t)
			o := f.backend(t)
			s := f.create(t, o)
			f.host.automation()
			prompt := Prompt{ID: uuid.NewString(), Text: "hello"}
			require.NoError(t, o.Prompt(t.Context(), s, prompt))
			nativeID := f.mutations()[1].body["id"].(string)
			f.mu.Lock()
			f.deleteDelivers = race
			f.mu.Unlock()
			err := o.Cancel(t.Context(), s, prompt.ID)
			if race {
				require.ErrorContains(t, err, "already delivered")
			} else {
				require.NoError(t, err)
				f.host.wait(t, func(events []Event) bool { return openCodeHasEvent(events, "prompt.rejected") })
				for _, event := range f.host.all() {
					if event.Type == "prompt.rejected" {
						//nolint:misspell // Preserve the normalized wire contract.
						require.Equal(t, true, event.Data["cancelled"])
					}
				}
				saved, err := f.host.Session(s.ID)
				require.NoError(t, err)
				receipts, err := readOpenCodeReceipts(saved.State)
				require.NoError(t, err)
				//nolint:misspell // Existing durable receipts use this spelling.
				require.Equal(t, "cancelled", receipts.Requests[prompt.ID].Status)
				require.NoError(t, o.Cancel(t.Context(), s, prompt.ID))
			}
			calls := f.mutations()
			require.Len(t, calls, 3)
			require.Equal(t, "DELETE", calls[2].method)
			require.Equal(t, "/api/session/"+f.id+"/inbox/"+nativeID, calls[2].path)
			for _, call := range calls {
				require.NotContains(t, call.path, "interrupt")
			}
		})
	}
}

func TestOpenCodeUnknownPromptNeverRetried(t *testing.T) {
	t.Parallel()
	f := newOpenCodeTestNative(t)
	o := f.backend(t)
	s := f.create(t, o)
	f.host.automation()
	prompt := Prompt{ID: uuid.NewString(), Text: "private prompt"}
	f.mu.Lock()
	f.promptStatus = 500
	f.mu.Unlock()
	err := o.Prompt(t.Context(), s, prompt)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "private-secret")
	require.NoError(t, o.Close())
	restored := f.backend(t)
	saved, err := f.host.Session(s.ID)
	require.NoError(t, err)
	require.NotContains(t, string(saved.State), prompt.Text)
	require.NoError(t, restored.Resume(t.Context(), saved))
	require.NoError(t, restored.Prompt(t.Context(), saved, prompt))
	require.Len(t, f.mutations(), 2)
}

func TestOpenCodePersistenceBeforeDispatch(t *testing.T) {
	t.Parallel()
	f := newOpenCodeTestNative(t)
	o := f.backend(t)
	s := f.create(t, o)
	f.host.automation()
	f.host.mu.Lock()
	f.host.fail = true
	f.host.mu.Unlock()
	err := o.Prompt(t.Context(), s, Prompt{ID: uuid.NewString(), Text: "hello"})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "private secret")
	require.Len(t, f.mutations(), 1)
}
