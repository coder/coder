package codersdk

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// StreamWorkloadOutput follows retained output without relaunching the command.
// Establishment is limited to ten seconds; the response body lives until ctx is
// canceled or the caller closes it. Requests are not retried.
//
// The caller must close the body and decode newline-delimited WorkloadOutput
// frames with a maximum size of 4 MiB, ignoring blank heartbeat lines. A terminal
// frame with no Events indicates that all retained output has been drained.
func (c *Client) StreamWorkloadOutput(ctx context.Context, org, id uuid.UUID, offset int64) (io.ReadCloser, error) {
	ctx = context.WithValue(ctx, privateRequestBodyKey{}, true)
	ctx, cancel := context.WithCancel(ctx)

	// Preserve the client's transport, authentication, and logging without
	// letting an overall HTTP timeout terminate a healthy stream.
	httpClient := *c.HTTPClient
	httpClient.Timeout = 0
	c.mu.RLock()
	streamClient := &Client{
		HTTPClient:           &httpClient,
		URL:                  c.URL,
		SessionTokenProvider: c.SessionTokenProvider,
		logger:               c.logger,
		logBodies:            c.logBodies,
		PlainLogger:          c.PlainLogger,
		Trace:                c.Trace,
	}
	c.mu.RUnlock()

	timer := time.AfterFunc(10*time.Second, cancel)
	res, err := streamClient.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/workloads/%s?offset=%d&follow=true", org, id, offset), nil)
	if !timer.Stop() {
		cancel()
		if res != nil {
			_ = res.Body.Close()
		}
		return nil, xerrors.Errorf("establish workload output stream: %w", context.DeadlineExceeded)
	}
	if err != nil {
		cancel()
		return nil, err
	}

	// Closing or canceling must release the body even with a custom transport.
	closeBody := sync.OnceValue(res.Body.Close)
	stopClose := context.AfterFunc(ctx, func() { _ = closeBody() })
	res.Body = struct {
		io.Reader
		io.Closer
	}{res.Body, closeFunc(func() error {
		stopClose()
		cancel()
		return closeBody()
	})}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		err := ReadBodyAsError(res)
		if res.StatusCode == http.StatusTooManyRequests {
			return nil, &workloadRateLimitError{err: err, retryAfter: workloadRetryAfter(res.Header)}
		}
		return nil, err
	}
	contentType := res.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "application/x-ndjson" {
		_ = res.Body.Close()
		return nil, xerrors.Errorf("unexpected workload output content type %q", contentType)
	}
	return res.Body, nil
}
