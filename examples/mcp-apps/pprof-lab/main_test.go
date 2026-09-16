package main_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pproflab "github.com/coder/coder/examples/mcp-apps/pprof-lab"
	"github.com/coder/coder/examples/mcp-apps/pprof-lab/lab"
)

type scenarioStatus struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Knobs       []lab.Knob     `json:"knobs"`
	Running     bool           `json:"running"`
	State       map[string]any `json:"state"`
}

type statusResponse struct {
	Scenarios     []scenarioStatus `json:"scenarios"`
	NumGoroutine  int              `json:"num_goroutine"`
	Mem           map[string]any   `json:"mem"`
	UptimeSeconds float64          `json:"uptime_seconds"`
	GoVersion     string           `json:"go_version"`
}

// newServer starts an httptest server over a fresh registry. Scenarios are
// stopped when the test ends.
func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	reg := lab.NewRegistry()
	srv := httptest.NewServer(pproflab.NewHandler(reg, time.Now()))
	t.Cleanup(func() {
		srv.Close()
		reg.StopAll()
	})
	return srv
}

// response is a fully read HTTP response.
type response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// do sends a request and returns the response with its body fully read.
func do(t *testing.T, srv *httptest.Server, method, path string, body any, headers map[string]string) response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, srv.URL+path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		if v == "" {
			req.Header.Del(k)
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return response{StatusCode: resp.StatusCode, Header: resp.Header, Body: data}
}

func decode[T any](t *testing.T, data []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("decode %q: %v", data, err)
	}
	return v
}

func getStatus(t *testing.T, srv *httptest.Server) statusResponse {
	t.Helper()
	resp := do(t, srv, http.MethodGet, "/api/status", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/status = %d: %s", resp.StatusCode, resp.Body)
	}
	return decode[statusResponse](t, resp.Body)
}

func findScenario(t *testing.T, st statusResponse, name string) scenarioStatus {
	t.Helper()
	for _, s := range st.Scenarios {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("scenario %q missing from status", name)
	return scenarioStatus{}
}

func TestIndex(t *testing.T) {
	t.Parallel()
	srv := newServer(t)
	resp := do(t, srv, http.MethodGet, "/", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", ct)
	}
	if !bytes.Contains(resp.Body, []byte("/api/status")) {
		t.Fatal("index page does not reference /api/status")
	}
}

func TestStatus(t *testing.T) {
	t.Parallel()
	srv := newServer(t)
	st := getStatus(t, srv)

	if len(st.Scenarios) != 6 {
		t.Fatalf("len(scenarios) = %d, want 6", len(st.Scenarios))
	}
	for _, s := range st.Scenarios {
		if s.Running {
			t.Fatalf("%s running at startup", s.Name)
		}
		if s.Description == "" || len(s.Knobs) == 0 {
			t.Fatalf("%s missing description or knobs", s.Name)
		}
	}
	if st.NumGoroutine <= 0 {
		t.Fatalf("num_goroutine = %d", st.NumGoroutine)
	}
	for _, key := range []string{"heap_alloc", "heap_inuse", "heap_objects", "num_gc"} {
		if _, ok := st.Mem[key]; !ok {
			t.Fatalf("mem missing %q", key)
		}
	}
	if st.UptimeSeconds < 0 {
		t.Fatalf("uptime_seconds = %v", st.UptimeSeconds)
	}
	if !strings.HasPrefix(st.GoVersion, "go") {
		t.Fatalf("go_version = %q", st.GoVersion)
	}
}

func TestStartStopReset(t *testing.T) {
	t.Parallel()
	srv := newServer(t)

	resp := do(t, srv, http.MethodPost, "/api/scenarios/goroutine-leak/start", map[string]any{"count": 3}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("start = %d: %s", resp.StatusCode, resp.Body)
	}
	started := decode[scenarioStatus](t, resp.Body)
	if !started.Running || started.State["goroutines"] != float64(3) {
		t.Fatalf("start response = %+v", started)
	}
	if s := findScenario(t, getStatus(t, srv), "goroutine-leak"); !s.Running {
		t.Fatal("status does not report goroutine-leak running")
	}

	// An empty body starts with defaults.
	resp = do(t, srv, http.MethodPost, "/api/scenarios/cpu-burn/start", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("start with empty body = %d: %s", resp.StatusCode, resp.Body)
	}

	resp = do(t, srv, http.MethodPost, "/api/scenarios/goroutine-leak/stop", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stop = %d: %s", resp.StatusCode, resp.Body)
	}
	if stopped := decode[scenarioStatus](t, resp.Body); stopped.Running {
		t.Fatalf("stop response still running: %+v", stopped)
	}

	resp = do(t, srv, http.MethodPost, "/api/reset", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reset = %d: %s", resp.StatusCode, resp.Body)
	}
	for _, s := range getStatus(t, srv).Scenarios {
		if s.Running {
			t.Fatalf("%s running after reset", s.Name)
		}
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		method  string
		path    string
		body    any
		headers map[string]string
		want    int
	}{
		{
			name:   "unknown scenario start",
			method: http.MethodPost, path: "/api/scenarios/nope/start",
			want: http.StatusNotFound,
		},
		{
			name:   "unknown scenario stop",
			method: http.MethodPost, path: "/api/scenarios/nope/stop",
			want: http.StatusNotFound,
		},
		{
			name:   "knob above max",
			method: http.MethodPost, path: "/api/scenarios/goroutine-leak/start",
			body: map[string]any{"count": 20001},
			want: http.StatusBadRequest,
		},
		{
			name:   "knob below min",
			method: http.MethodPost, path: "/api/scenarios/heap-growth/start",
			body: map[string]any{"cap_mib": 0},
			want: http.StatusBadRequest,
		},
		{
			name:   "unknown knob",
			method: http.MethodPost, path: "/api/scenarios/cpu-burn/start",
			body: map[string]any{"threads": 1},
			want: http.StatusBadRequest,
		},
		{
			name:   "non-object body",
			method: http.MethodPost, path: "/api/scenarios/cpu-burn/start",
			body: []int{1, 2},
			want: http.StatusBadRequest,
		},
		{
			name:   "missing content type on start",
			method: http.MethodPost, path: "/api/scenarios/goroutine-leak/start",
			body:    map[string]any{"count": 1},
			headers: map[string]string{"Content-Type": ""},
			want:    http.StatusUnsupportedMediaType,
		},
		{
			name:   "form content type on stop",
			method: http.MethodPost, path: "/api/scenarios/goroutine-leak/stop",
			headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
			want:    http.StatusUnsupportedMediaType,
		},
		{
			name:   "missing content type on reset",
			method: http.MethodPost, path: "/api/reset",
			headers: map[string]string{"Content-Type": ""},
			want:    http.StatusUnsupportedMediaType,
		},
		{
			name:   "cross-site start",
			method: http.MethodPost, path: "/api/scenarios/goroutine-leak/start",
			body:    map[string]any{"count": 1},
			headers: map[string]string{"Sec-Fetch-Site": "cross-site"},
			want:    http.StatusForbidden,
		},
		{
			name:   "cross-site reset",
			method: http.MethodPost, path: "/api/reset",
			headers: map[string]string{"Sec-Fetch-Site": "cross-site"},
			want:    http.StatusForbidden,
		},
		{
			name:   "same-origin is allowed",
			method: http.MethodPost, path: "/api/scenarios/goroutine-leak/stop",
			headers: map[string]string{"Sec-Fetch-Site": "same-origin"},
			want:    http.StatusOK,
		},
		{
			name:   "get on mutating route",
			method: http.MethodGet, path: "/api/reset",
			want: http.StatusMethodNotAllowed,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := newServer(t)
			resp := do(t, srv, tc.method, tc.path, tc.body, tc.headers)
			if resp.StatusCode != tc.want {
				t.Fatalf("%s %s = %d, want %d: %s", tc.method, tc.path, resp.StatusCode, tc.want, resp.Body)
			}
			if tc.want >= 400 && tc.want != http.StatusMethodNotAllowed {
				body := decode[map[string]string](t, resp.Body)
				if body["error"] == "" {
					t.Fatalf("error body missing: %s", resp.Body)
				}
			}
			for _, s := range getStatus(t, srv).Scenarios {
				if s.Running {
					t.Fatalf("%s running after rejected request", s.Name)
				}
			}
		})
	}
}

func TestPprofHeap(t *testing.T) {
	t.Parallel()
	srv := newServer(t)

	resp := do(t, srv, http.MethodGet, "/debug/pprof/heap", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /debug/pprof/heap = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/octet-stream" {
		t.Fatalf("Content-Type = %q, want application/octet-stream", ct)
	}
	data := resp.Body
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		t.Fatalf("body is not gzip (first bytes % x)", data[:min(len(data), 4)])
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("decoded profile is empty")
	}
}

func TestPprofIndex(t *testing.T) {
	t.Parallel()
	srv := newServer(t)
	resp := do(t, srv, http.MethodGet, "/debug/pprof/", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /debug/pprof/ = %d", resp.StatusCode)
	}
	for _, name := range []string{"goroutine", "heap", "mutex", "block", "profile"} {
		if !bytes.Contains(resp.Body, []byte(name)) {
			t.Fatalf("pprof index does not list %q", name)
		}
	}
}
