package workspacesdk

import (
	"io"
	"net/http"
	neturl "net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

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

// countingReader serves filler bytes after a prefix, up to size bytes,
// and records how many bytes the consumer pulled.
type countingReader struct {
	prefix string
	size   int64
	read   int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	if r.read >= r.size {
		return 0, io.EOF
	}
	n := 0
	for n < len(p) && r.read < r.size {
		if int(r.read) < len(r.prefix) {
			p[n] = r.prefix[r.read]
		} else {
			p[n] = 'a'
		}
		n++
		r.read++
	}
	return n, nil
}

func TestDecodeAgentJSONRejectsOversizedBody(t *testing.T) {
	t.Parallel()

	body := &countingReader{prefix: `{"output":"`, size: 2 * agentJSONResponseMaxBytes}
	res := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(body),
	}

	var out DesktopActionResponse
	err := decodeAgentJSON(res, &out)
	require.ErrorContains(t, err, "agent response exceeds")
	require.LessOrEqual(t, body.read, agentJSONResponseMaxBytes+1)
}

func TestReadAgentErrorBoundsBody(t *testing.T) {
	t.Parallel()

	body := &countingReader{prefix: `{"message":"`, size: 1 << 20}
	res := &http.Response{
		StatusCode: http.StatusConflict,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(body),
	}

	err := readAgentError(res)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusConflict, sdkErr.StatusCode())
	require.LessOrEqual(t, body.read, agentErrorResponseMaxBytes)
}
