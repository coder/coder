package codersdk_test

import (
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestWebsocketNetConn_LargeWrites tests that we can write large amounts of data thru the netconn
// in a single write.  Without specifically setting the read limit, the websocket library limits
// the amount of data that can be read in a single message to 32kiB.  Even after raising the limit,
// curiously, it still only reads 32kiB per Read(), but allows the large write to go thru.
func TestWebsocketNetConn_LargeWrites(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	n := 4 * 1024 * 1024 // 4 MiB
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sws, err := websocket.Accept(w, r, nil)
		if !assert.NoError(t, err) {
			return
		}
		_, nc := codersdk.WebsocketNetConn(r.Context(), sws, websocket.MessageBinary)
		defer nc.Close()

		// Although the writes are all in one go, the reads get broken up by
		// the library.
		j := 0
		b := make([]byte, n)
		for j < n {
			k, err := nc.Read(b[j:])
			if !assert.NoError(t, err) {
				return
			}
			j += k
			t.Logf("server read %d bytes, total %d", k, j)
		}
		assert.Equal(t, n, j)
		j, err = nc.Write(b)
		assert.Equal(t, n, j)
		if !assert.NoError(t, err) {
			return
		}
	}))

	// use of random data is worst case scenario for compression
	cb := make([]byte, n)
	rk, err := rand.Read(cb)
	require.NoError(t, err)
	require.Equal(t, n, rk)

	// nolint: bodyclose
	cws, _, err := websocket.Dial(ctx, svr.URL, nil)
	require.NoError(t, err)
	_, cnc := codersdk.WebsocketNetConn(ctx, cws, websocket.MessageBinary)
	ck, err := cnc.Write(cb)
	require.NoError(t, err)
	require.Equal(t, n, ck)

	cb2 := make([]byte, n)
	j := 0
	for j < n {
		k, err := cnc.Read(cb2[j:])
		if !assert.NoError(t, err) {
			return
		}
		j += k
		t.Logf("client read %d bytes, total %d", k, j)
	}
	require.NoError(t, err)
	require.Equal(t, n, j)
	require.Equal(t, cb, cb2)
}
