package codersdk

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/xerrors"
)

// DebugProfileDurationMax is the maximum duration the server will accept
// for a profile collection. Callers should ensure their context deadline
// exceeds this to avoid premature cancellation.
const DebugProfileDurationMax = 60 * time.Second

// DebugProfileOptions are options for collecting debug profiles from the
// server via the consolidated /debug/profile endpoint.
type DebugProfileOptions struct {
	// Duration controls how long time-based profiles (cpu, trace) run.
	// Zero uses the server default (10s).
	Duration time.Duration
	// Profiles is the list of profile types to collect. Nil or empty uses
	// the server default (cpu, heap, allocs, block, mutex, goroutine).
	Profiles []string
}

// DebugCollectProfile fetches a tar.gz archive of pprof profiles from the
// server. The caller is responsible for closing the returned ReadCloser.
func (c *Client) DebugCollectProfile(ctx context.Context, opts DebugProfileOptions) (io.ReadCloser, error) {
	qp := url.Values{}
	if opts.Duration > 0 {
		qp.Set("duration", opts.Duration.String())
	}
	if len(opts.Profiles) > 0 {
		qp.Set("profiles", strings.Join(opts.Profiles, ","))
	}

	reqPath := "/api/v2/debug/profile"
	if len(qp) > 0 {
		reqPath += "?" + qp.Encode()
	}

	resp, err := c.Request(ctx, http.MethodPost, reqPath, nil)
	if err != nil {
		return nil, xerrors.Errorf("request debug profile: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, ReadBodyAsError(resp)
	}

	return resp.Body, nil
}
