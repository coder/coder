package agentnative

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/xerrors"

	"github.com/coder/quartz"
)

const (
	openCodeVersion     = "2.0.28"
	openCodeMaxResponse = 4 << 20
	openCodeMaxEvent    = 256 << 10
)

var (
	openCodeSessionID   = regexp.MustCompile(`^ses_[a-f0-9]{12}[0-9A-Za-z]{14}$`)
	openCodeMessageID   = regexp.MustCompile(`^msg_[a-f0-9]{12}[0-9A-Za-z]{14}$`)
	openCodeDigest      = regexp.MustCompile(`^[a-f0-9]{64}$`)
	errOpenCodeProtocol = xerrors.New("invalid or unbounded OpenCode 2.0.28 response")
	errOpenCodeUnknown  = xerrors.New("OpenCode request outcome is unknown; inspect the native terminal before retrying")
	errOpenCodeAnchor   = xerrors.New("OpenCode bounded history anchor is missing; no work was replayed")
)

type openCodeRPCError struct{ rejected bool }

func (*openCodeRPCError) Error() string { return "native OpenCode request rejected" }

// Native errors and transport causes can contain credentials or private input.
// Only locally defined errors cross this boundary.
func openCodeHTTPClient() *http.Client {
	return &http.Client{
		Transport:     &http.Transport{Proxy: nil, DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func openCodeTimeout(ctx context.Context, clock quartz.Clock, duration time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	timer := clock.AfterFunc(duration, cancel, "opencode.timeout")
	return ctx, func() { timer.Stop(); cancel() }
}

type openCodeNativeEvent struct {
	ID   string                     `json:"id"`
	Type string                     `json:"type"`
	Data map[string]json.RawMessage `json:"data"`
}

type openCodeRPC struct {
	endpoint string
	password string
	http     *http.Client
	clock    quartz.Clock
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
}

func openCodeEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	return err == nil && u.Scheme == "http" && u.Hostname() == "127.0.0.1" && u.Port() != "" &&
		u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == "" && u.Opaque == ""
}

func (c *openCodeRPC) open(ctx context.Context, method, path string, body any, accept string) (*http.Response, error) {
	if !strings.HasPrefix(path, "/api/") || strings.ContainsAny(path, "\r\n#\\") {
		return nil, errOpenCodeProtocol
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil || len(encoded) > openCodeMaxResponse {
			return nil, errOpenCodeProtocol
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, reader)
	if err != nil {
		return nil, errOpenCodeProtocol
	}
	req.SetBasicAuth("opencode", c.password)
	req.Header.Set("Accept", accept)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, errOpenCodeUnknown
	}
	return res, nil
}

func (c *openCodeRPC) request(ctx context.Context, method, path string, body, result any) error {
	ctx, cancel := openCodeTimeout(ctx, c.clock, 10*time.Second)
	defer cancel()
	stop := context.AfterFunc(c.ctx, cancel)
	defer stop()
	if c.ctx.Err() != nil {
		return errOpenCodeUnknown
	}
	res, err := c.open(ctx, method, path, body, "application/json")
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNoContent {
		if result != nil {
			return errOpenCodeProtocol
		}
		return nil
	}
	if res.StatusCode != http.StatusOK {
		switch res.StatusCode {
		case 400, 401, 403, 404, 409, 422:
			return &openCodeRPCError{rejected: true}
		default:
			return errOpenCodeUnknown
		}
	}
	kind, _, err := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if err != nil || kind != "application/json" {
		return errOpenCodeProtocol
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, openCodeMaxResponse+1))
	if err != nil || len(data) > openCodeMaxResponse || !utf8.Valid(data) || !json.Valid(data) {
		return errOpenCodeProtocol
	}
	if result != nil && json.Unmarshal(data, result) != nil {
		return errOpenCodeProtocol
	}
	return nil
}

// SSE only invalidates snapshots. It never supplies assistant text or ordering.
func (c *openCodeRPC) subscribe(ready chan<- error, emit func(openCodeNativeEvent)) {
	defer close(c.done)
	connected := false
	defer func() {
		if !connected {
			ready <- xerrors.New("OpenCode event transport disconnected")
		}
		if c.ctx.Err() == nil {
			c.cancel()
			emit(openCodeNativeEvent{Type: "transport.lost"})
		}
	}()
	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()
	idle := c.clock.AfterFunc(45*time.Second, cancel, "opencode.sse.idle")
	defer idle.Stop()
	res, err := c.open(ctx, http.MethodGet, "/api/event", nil, "text/event-stream")
	if err != nil {
		return
	}
	defer res.Body.Close()
	kind, _, err := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if err != nil || kind != "text/event-stream" || res.StatusCode != 200 {
		return
	}
	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 4096), openCodeMaxEvent+1)
	var data strings.Builder
	frameSize := 0
	failure := false
	for scanner.Scan() {
		idle.Reset(45 * time.Second)
		line := scanner.Text()
		if !utf8.ValidString(line) {
			return
		}
		frameSize += len(line)
		if frameSize > openCodeMaxEvent {
			return
		}
		if line != "" {
			if strings.HasPrefix(line, "event:") && strings.TrimSpace(strings.TrimPrefix(line, "event:")) == "effect/http-api/stream/failure" {
				failure = true
			}
			if value, ok := strings.CutPrefix(line, "data:"); ok {
				if data.Len() != 0 {
					_ = data.WriteByte('\n')
				}
				_, _ = data.WriteString(strings.TrimPrefix(value, " "))
			}
			continue
		}
		if failure {
			return
		}
		frameSize = 0
		if data.Len() == 0 {
			continue
		}
		var event openCodeNativeEvent
		if json.Unmarshal([]byte(data.String()), &event) != nil || event.ID == "" || len(event.ID) > 200 || event.Type == "" || event.Data == nil {
			return
		}
		data.Reset()
		if !connected {
			if event.Type != "server.connected" {
				return
			}
			connected = true
			ready <- nil
		} else {
			if event.Type == "server.connected" {
				return
			}
			emit(event)
		}
	}
}

func newOpenCodeRPC(ctx, lifetime context.Context, d openCodeDescriptor, clock quartz.Clock, client *http.Client, emit func(openCodeNativeEvent)) (*openCodeRPC, error) {
	if !openCodeEndpoint(d.URL) || !openCodeDigest.MatchString(d.Password) {
		return nil, errOpenCodeProtocol
	}
	lifetime, cancel := context.WithCancel(lifetime)
	c := &openCodeRPC{endpoint: d.URL, password: d.Password, http: client, clock: clock, ctx: lifetime, cancel: cancel, done: make(chan struct{})}
	var info struct {
		Version string `json:"version"`
	}
	if err := c.request(ctx, "GET", "/api/info", nil, &info); err != nil {
		cancel()
		return nil, err
	}
	if info.Version != openCodeVersion {
		cancel()
		return nil, xerrors.New("OpenCode requires exact version 2.0.28")
	}
	ready := make(chan error, 1)
	go c.subscribe(ready, emit)
	wait, stop := openCodeTimeout(ctx, clock, 10*time.Second)
	defer stop()
	select {
	case err := <-ready:
		if err == nil {
			return c, nil
		}
	case <-wait.Done():
	}
	c.close()
	return nil, xerrors.New("OpenCode event transport not ready")
}

func (c *openCodeRPC) close() { c.cancel(); <-c.done }
