package workspacesdk

import (
	"context"
	neturl "net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeProberConn struct {
	AgentConn
	probed string
}

func (c *fakeProberConn) ProcessByToken(_ context.Context, token string) (ProcessByTokenResponse, error) {
	c.probed = token
	return ProcessByTokenResponse{Found: true, ProcessID: "proc-1"}, nil
}

func TestProbeProcessToken(t *testing.T) {
	t.Parallel()

	t.Run("unsupported connection degrades", func(t *testing.T) {
		t.Parallel()
		// A bare interface embed satisfies AgentConn without the
		// optional prober capability.
		conn := struct{ AgentConn }{}
		_, err := ProbeProcessToken(context.Background(), conn, "tok")
		require.ErrorIs(t, err, ErrProcessTokenProbeUnsupported)
	})

	t.Run("wrapping preserves the capability", func(t *testing.T) {
		t.Parallel()
		inner := &fakeProberConn{}
		wrapped := WrapAgentConn(inner, nil)
		resp, err := ProbeProcessToken(context.Background(), wrapped, "tok")
		require.NoError(t, err)
		require.True(t, resp.Found)
		require.Equal(t, "tok", inner.probed)
	})

	t.Run("wrapping an unsupported connection degrades", func(t *testing.T) {
		t.Parallel()
		wrapped := WrapAgentConn(struct{ AgentConn }{}, nil)
		_, err := ProbeProcessToken(context.Background(), wrapped, "tok")
		require.ErrorIs(t, err, ErrProcessTokenProbeUnsupported)
	})
}

func TestAgentAPIPath(t *testing.T) {
	t.Parallel()

	t.Run("encodes reserved query characters", func(t *testing.T) {
		t.Parallel()

		path := "/tmp/a&b ?#%c.md"
		got := agentAPIPath("/api/v0/resolve-path", neturl.Values{
			"path": []string{path},
		})

		parsed, err := neturl.Parse(got)
		require.NoError(t, err)
		require.Equal(t, "/api/v0/resolve-path", parsed.Path)
		require.Equal(t, path, parsed.Query().Get("path"))
	})

	t.Run("preserves all query values", func(t *testing.T) {
		t.Parallel()

		got := agentAPIPath("/api/v0/read-file-lines", neturl.Values{
			"path":               []string{"/tmp/plan v1#.md"},
			"offset":             []string{"10"},
			"limit":              []string{"20"},
			"max_file_size":      []string{"30"},
			"max_line_bytes":     []string{"40"},
			"max_response_lines": []string{"50"},
			"max_response_bytes": []string{"60"},
		})

		parsed, err := neturl.Parse(got)
		require.NoError(t, err)
		require.Equal(t, "/api/v0/read-file-lines", parsed.Path)
		require.Equal(t, "/tmp/plan v1#.md", parsed.Query().Get("path"))
		require.Equal(t, "10", parsed.Query().Get("offset"))
		require.Equal(t, "20", parsed.Query().Get("limit"))
		require.Equal(t, "30", parsed.Query().Get("max_file_size"))
		require.Equal(t, "40", parsed.Query().Get("max_line_bytes"))
		require.Equal(t, "50", parsed.Query().Get("max_response_lines"))
		require.Equal(t, "60", parsed.Query().Get("max_response_bytes"))
	})

	t.Run("debug logs zero after", func(t *testing.T) {
		t.Parallel()

		got := debugLogsPath(time.Time{})
		require.Equal(t, "/debug/logs", got)
	})

	t.Run("debug logs after", func(t *testing.T) {
		t.Parallel()

		after := time.Date(2026, 5, 18, 12, 34, 56, 789, time.FixedZone("test", -7*60*60))
		got := debugLogsPath(after)
		parsed, err := neturl.Parse(got)
		require.NoError(t, err)
		require.Equal(t, "/debug/logs", parsed.Path)
		require.Equal(t, after.UTC().Format(time.RFC3339Nano), parsed.Query().Get("after"))
	})
}
