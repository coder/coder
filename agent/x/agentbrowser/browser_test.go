package agentbrowser_test

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/x/agentbrowser"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

func findChrome(t *testing.T) {
	t.Helper()
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
		if _, err := exec.LookPath(name); err == nil {
			return
		}
	}
	t.Skip("Chrome is not installed")
}

type streamClient struct {
	t  *testing.T
	nc io.ReadWriteCloser
}

func (c *streamClient) send(msg map[string]any) {
	c.t.Helper()
	data, err := json.Marshal(msg)
	require.NoError(c.t, err)
	frame := make([]byte, 5+len(data))
	frame[0] = 2
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(data))) //nolint:gosec // Test payloads are small.
	copy(frame[5:], data)
	_, err = c.nc.Write(frame)
	require.NoError(c.t, err)
}

// next returns the next frame type and payload.
func (c *streamClient) next() (byte, []byte) {
	c.t.Helper()
	var hdr [5]byte
	_, err := io.ReadFull(c.nc, hdr[:])
	require.NoError(c.t, err)
	payload := make([]byte, binary.BigEndian.Uint32(hdr[1:5]))
	_, err = io.ReadFull(c.nc, payload)
	require.NoError(c.t, err)
	return hdr[0], payload
}

// TestBrowserStream runs a real headless Chrome and checks that the
// stream delivers video, follows navigation and rejects non-URL input.
func TestBrowserStream(t *testing.T) {
	t.Parallel()
	findChrome(t)

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	site := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(rw, `<!doctype html><title>Page %s</title><h1>%s</h1>
<script>let n=0;setInterval(()=>{document.querySelector('h1').textContent='tick '+n++},50)</script>`, r.URL.Path, r.URL.Path)
	}))
	t.Cleanup(site.Close)

	browser := agentbrowser.New(agentbrowser.Options{
		Logger:     logger,
		ProfileDir: t.TempDir(),
	})
	t.Cleanup(func() { _ = browser.Close() })
	api := agentbrowser.NewAPI(logger, browser)
	srv := httptest.NewServer(api.Routes())
	t.Cleanup(srv.Close)

	conn, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/stream", nil)
	require.NoError(t, err)
	if res.Body != nil {
		defer res.Body.Close()
	}
	conn.SetReadLimit(-1)
	nc := websocket.NetConn(ctx, conn, websocket.MessageBinary)
	t.Cleanup(func() { _ = nc.Close() })
	c := &streamClient{t: t, nc: nc}

	c.send(map[string]any{"type": "resize", "width": 800, "height": 600})
	c.send(map[string]any{"type": "navigate", "url": site.URL + "/one"})

	var (
		sawConfig bool
		sawKey    bool
		deltas    int
		lastURL   string
	)
	deadline := time.Now().Add(testutil.WaitLong)
	for time.Now().Before(deadline) && (!sawConfig || !sawKey || deltas < 5 || !strings.HasSuffix(lastURL, "/one")) {
		typ, payload := c.next()
		switch typ {
		case 1:
			if payload[0]&1 != 0 {
				sawKey = true
			} else if sawKey {
				deltas++
			}
		case 2:
			var msg struct {
				Type    string `json:"type"`
				Codec   string `json:"codec"`
				URL     string `json:"url"`
				Message string `json:"message"`
			}
			require.NoError(t, json.Unmarshal(payload, &msg))
			switch msg.Type {
			case "config":
				sawConfig = true
				t.Logf("config: %s", payload)
			case "state":
				lastURL = msg.URL
			case "error":
				t.Fatalf("server error: %s", msg.Message)
			}
		}
	}
	require.True(t, sawConfig, "config")
	require.True(t, sawKey, "keyframe")
	require.GreaterOrEqual(t, deltas, 5, "delta frames")
	require.True(t, strings.HasSuffix(lastURL, "/one"), "navigated, last url %q", lastURL)

	c.send(map[string]any{"type": "navigate", "url": "search terms"})
	for {
		typ, payload := c.next()
		if typ != 2 {
			continue
		}
		var msg struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		}
		require.NoError(t, json.Unmarshal(payload, &msg))
		if msg.Type == "error" {
			require.Contains(t, msg.Message, "URL")
			break
		}
	}
}
