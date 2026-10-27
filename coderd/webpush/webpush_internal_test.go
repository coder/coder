package webpush

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestHTTPClientLimits(t *testing.T) {
	t.Parallel()
	client := newHTTPClient()
	require.Equal(t, pushRequestTimeout, client.Timeout)
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.Equal(t, int64(maxResponseHeaderBytes), transport.MaxResponseHeaderBytes)
	require.NotNil(t, transport.Proxy, "the default transport's proxy settings must be kept")
}

// TestStalledServerTimesOut reproduces a push endpoint that accepts the
// connection and never responds. The client timeout must end the call.
func TestStalledServerTimesOut(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	// Cleanups run in reverse order. The server's Close waits for the
	// handler to return, so the handler must be released first.
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	client := server.Client()
	client.Timeout = time.Second

	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	require.NoError(t, err)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	pusher := &Webpusher{
		log:             &logger,
		vapidSub:        "http://example.com",
		httpClient:      client,
		VAPIDPublicKey:  publicKey,
		VAPIDPrivateKey: privateKey,
	}

	err = pusher.Test(ctx, codersdk.WebpushSubscription{
		Endpoint:  server.URL,
		AuthKey:   "zqbxT6JKstKSY9JKibZLSQ==",
		P256DHKey: "BNNL5ZaTfK81qhXOx23+wewhigUeFb632jN6LvRWCFH1ubQr77FE/9qV1FuojuRmHP42zmf34rXgW80OvUVDgTk=",
	})
	require.Error(t, err)
	var netErr net.Error
	require.ErrorAs(t, err, &netErr)
	require.True(t, netErr.Timeout(), "expected a timeout, got: %v", err)
	require.NoError(t, ctx.Err(), "the client timeout, not the test context, should end the call")
}
