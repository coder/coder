package healthcheck

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/coderd/util/ptr"
)

// @typescript-generate AccessURLReport
type AccessURLReport struct {
	// Healthy is deprecated and left for backward compatibility purposes, use `Severity` instead.
	Healthy  bool            `json:"healthy"`
	Severity health.Severity `json:"severity" enums:"ok,warning,error"`
	Warnings []string        `json:"warnings"`

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

	r.Severity = health.SeverityOK
	r.Warnings = []string{}
	if opts.AccessURL == nil {
		r.Error = ptr.Ref("access URL is nil")
		r.Severity = health.SeverityError
		return
	}
	r.AccessURL = opts.AccessURL.String()

	if opts.Client == nil {
		opts.Client = http.DefaultClient
	}

	accessURL, err := opts.AccessURL.Parse("/healthz")
	if err != nil {
		r.Error = convertError(xerrors.Errorf("parse healthz endpoint: %w", err))
		r.Severity = health.SeverityError
		return
	}

	req, err := http.NewRequestWithContext(ctx, "GET", accessURL.String(), nil)
	if err != nil {
		r.Error = convertError(xerrors.Errorf("create healthz request: %w", err))
		r.Severity = health.SeverityError
		return
	}

	res, err := opts.Client.Do(req)
	if err != nil {
		r.Error = convertError(xerrors.Errorf("get healthz endpoint: %w", err))
		r.Severity = health.SeverityError
		return
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		r.Error = convertError(xerrors.Errorf("read healthz response: %w", err))
		r.Severity = health.SeverityError
		return
	}

	r.Reachable = true
	r.Healthy = res.StatusCode == http.StatusOK
	r.StatusCode = res.StatusCode
	if res.StatusCode != http.StatusOK {
		r.Severity = health.SeverityWarning
	}
	r.HealthzResponse = string(body)
}
