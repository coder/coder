package codersdk

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// DialWorkloadInput connects to a workload's stdin without replaying input or
// restarting the workload. Send JSON WorkloadInput messages; output is available
// separately through StreamWorkloadOutput. The caller must read the connection
// to handle control frames and close it when finished. Closing the connection
// does not cancel the workload. Canceling ctx closes the connection.
//
// Only the handshake has a ten-second timeout. HTTP failures are returned as
// *Error; rate-limit errors additionally implement RetryAfter() time.Duration.
func (c *Client) DialWorkloadInput(ctx context.Context, org, id uuid.UUID) (*websocket.Conn, error) {
	u, err := c.URL.Parse(fmt.Sprintf("/api/v2/organizations/%s/workloads/%s/input", org, id))
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	// Keep header-command, custom TLS, and authentication transports, but do
	// not let the shared HTTP client's timeout bound the connection lifetime.
	httpClient := *c.HTTPClient
	httpClient.Timeout = 0
	transport := httpClient.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	httpClient.Transport = workloadInputTransport{base: transport, cancel: cancel}
	opts := &websocket.DialOptions{HTTPClient: &httpClient}
	c.mu.RLock()
	provider := c.SessionTokenProvider
	c.mu.RUnlock()
	provider.SetDialOption(opts)

	// Dial directly rather than using Request, which can log response bodies.
	timer := time.AfterFunc(10*time.Second, cancel)
	conn, res, err := websocket.Dial(ctx, u.String(), opts)
	if !timer.Stop() {
		cancel()
		if conn != nil {
			_ = conn.CloseNow()
		}
		return nil, xerrors.Errorf("establish workload input: %w", context.DeadlineExceeded)
	}
	if err != nil {
		defer cancel()
		if res != nil && res.StatusCode != http.StatusSwitchingProtocols {
			err := ReadBodyAsError(res)
			if res.StatusCode == http.StatusTooManyRequests {
				return nil, &workloadRateLimitError{err: err, retryAfter: workloadRetryAfter(res.Header)}
			}
			return nil, err
		}
		return nil, err
	}
	conn.SetReadLimit(32 << 10)
	// Closing the upgraded body cancels this child context, so this callback
	// is released on caller close as well as parent cancellation. No reader
	// goroutine competes with the caller or waits on a long-lived parent.
	context.AfterFunc(ctx, func() { _ = conn.CloseNow() })
	return conn, nil
}

type workloadInputTransport struct {
	base   http.RoundTripper
	cancel context.CancelFunc
}

func (t workloadInputTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	res, err := t.base.RoundTrip(req)
	if err == nil && res.StatusCode == http.StatusSwitchingProtocols {
		if body, ok := res.Body.(io.ReadWriteCloser); ok {
			res.Body = &workloadInputBody{ReadWriteCloser: body, cancel: t.cancel}
		}
	}
	return res, err
}

type workloadInputBody struct {
	io.ReadWriteCloser
	cancel context.CancelFunc
}

func (b *workloadInputBody) Close() error {
	b.cancel()
	return b.ReadWriteCloser.Close()
}
