package healthcheck

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/util/ptr"
)

// @typescript-generate AccessURLReport
type AccessURLReport struct {
	Healthy  bool     `json:"healthy"`
	Warnings []string `json:"warnings"`

	AccessURL       string  `json:"access_url"`
	Reachable       bool    `json:"reachable"`
	StatusCode      int     `json:"status_code"`
	HealthzResponse string  `json:"healthz_response"`
	Error           *string `json:"error"`
}

type AccessURLReportOptions struct {
	AccessURL *url.URL
	Client    *http.Client
}

func (r *AccessURLReport) Run(ctx context.Context, opts *AccessURLReportOptions) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	r.Warnings = []string{}
	if opts.AccessURL == nil {
		r.Error = ptr.Ref("access URL is nil")
		return
	}
	r.AccessURL = opts.AccessURL.String()

	if opts.Client == nil {
		opts.Client = http.DefaultClient
	}

	accessURL, err := opts.AccessURL.Parse("/healthz")
	if err != nil {
		r.Error = convertError(xerrors.Errorf("parse healthz endpoint: %w", err))
		return
	}

	req, err := http.NewRequestWithContext(ctx, "GET", accessURL.String(), nil)
	if err != nil {
		r.Error = convertError(xerrors.Errorf("create healthz request: %w", err))
		return
	}

	res, err := opts.Client.Do(req)
	if err != nil {
		r.Error = convertError(xerrors.Errorf("get healthz endpoint: %w", err))
		return
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		r.Error = convertError(xerrors.Errorf("read healthz response: %w", err))
		return
	}

	r.Reachable = true
	r.Healthy = res.StatusCode == http.StatusOK
	r.StatusCode = res.StatusCode
	r.HealthzResponse = string(body)
}
