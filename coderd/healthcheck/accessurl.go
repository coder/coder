package healthcheck

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/coderd/util/ptr"
)

// @typescript-generate AccessURLReport
type AccessURLReport struct {
	// Healthy is deprecated and left for backward compatibility purposes, use `Severity` instead.
	Healthy   bool            `json:"healthy"`
	Severity  health.Severity `json:"severity" enums:"ok,warning,error"`
	Warnings  []string        `json:"warnings"`
	Dismissed bool            `json:"dismissed"`

	AccessURL       string  `json:"access_url"`
	Reachable       bool    `json:"reachable"`
	StatusCode      int     `json:"status_code"`
	HealthzResponse string  `json:"healthz_response"`
	Error           *string `json:"error"`
}

type AccessURLReportOptions struct {
	AccessURL *url.URL
	Client    *http.Client

	Dismissed bool
}

func (r *AccessURLReport) Run(ctx context.Context, opts *AccessURLReportOptions) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	r.Severity = health.SeverityOK
	r.Warnings = []string{}
	r.Dismissed = opts.Dismissed

	if opts.AccessURL == nil {
		r.Error = ptr.Ref(health.Messagef(health.CodeAccessURLNotSet, "Access URL not set"))
		r.Severity = health.SeverityError
		return
	}
	r.AccessURL = opts.AccessURL.String()

	if opts.Client == nil {
		opts.Client = http.DefaultClient
	}

	accessURL, err := opts.AccessURL.Parse("/healthz")
	if err != nil {
		r.Error = ptr.Ref(health.Messagef(health.CodeAccessURLInvalid, "parse healthz endpoint: %s", err))
		r.Severity = health.SeverityError
		return
	}

	req, err := http.NewRequestWithContext(ctx, "GET", accessURL.String(), nil)
	if err != nil {
		r.Error = ptr.Ref(health.Messagef(health.CodeAccessURLFetch, "create healthz request: %s", err))
		r.Severity = health.SeverityError
		return
	}

	res, err := opts.Client.Do(req)
	if err != nil {
		r.Error = ptr.Ref(health.Messagef(health.CodeAccessURLFetch, "get healthz endpoint: %s", err))
		r.Severity = health.SeverityError
		return
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		r.Error = ptr.Ref(health.Messagef(health.CodeAccessURLFetch, "read healthz response: %s", err))
		r.Severity = health.SeverityError
		return
	}

	r.Reachable = true
	r.Healthy = res.StatusCode == http.StatusOK
	r.StatusCode = res.StatusCode
	if res.StatusCode != http.StatusOK {
		r.Severity = health.SeverityWarning
		r.Warnings = append(r.Warnings, health.Messagef(health.CodeAccessURLNotOK, "/healthz did not return 200 OK"))
	}
	r.HealthzResponse = string(body)
}
