//nolint:testpackage // This test exercises the internal query builder directly because agent requests need a live tailnet connection.
package workspacesdk

import (
	neturl "net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

type closeTrackingAgentConn struct {
	AgentConn
	closeCalls int
	closeErr   error
}

func (c *closeTrackingAgentConn) Close() error {
	c.closeCalls++
	return c.closeErr
}

func TestWrapAgentConn(t *testing.T) {
	t.Parallel()

	t.Run("nil conn runs close func", func(t *testing.T) {
		t.Parallel()

		closeCalls := 0
		wrapped := WrapAgentConn(nil, func() error {
			closeCalls++
			return nil
		})

		require.Nil(t, wrapped)
		require.Equal(t, 1, closeCalls)
	})

	t.Run("nil close func still wraps conn", func(t *testing.T) {
		t.Parallel()

		baseConn := &closeTrackingAgentConn{}
		wrapped := WrapAgentConn(baseConn, nil)
		require.NotNil(t, wrapped)
		require.NoError(t, wrapped.Close())
		require.Equal(t, 1, baseConn.closeCalls)
	})

	t.Run("close is idempotent and joins errors", func(t *testing.T) {
		t.Parallel()

		baseErr := xerrors.New("close conn")
		releaseErr := xerrors.New("release conn")
		baseConn := &closeTrackingAgentConn{closeErr: baseErr}
		releaseCalls := 0
		wrapped := WrapAgentConn(baseConn, func() error {
			releaseCalls++
			return releaseErr
		})

		err := wrapped.Close()
		require.ErrorIs(t, err, baseErr)
		require.ErrorIs(t, err, releaseErr)

		err = wrapped.Close()
		require.ErrorIs(t, err, baseErr)
		require.ErrorIs(t, err, releaseErr)
		require.Equal(t, 1, baseConn.closeCalls)
		require.Equal(t, 1, releaseCalls)
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
}
