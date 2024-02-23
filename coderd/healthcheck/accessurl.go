package healthcheck

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/codersdk"
)

type AccessURLReport codersdk.AccessURLReport

type AccessURLReportOptions struct {
	AccessURL *url.URL
	Client    *http.Client

	Dismissed bool
}

func (r *AccessURLReport) Run(ctx context.Context, opts *AccessURLReportOptions) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	r.Severity = health.SeverityOK
	r.Warnings = []health.Message{}
	r.Dismissed = opts.Dismissed

	if opts.AccessURL == nil {
		r.Error = health.Errorf(health.CodeAccessURLNotSet, "Access URL not set")
		r.Severity = health.SeverityError
		return
	}
	r.AccessURL = opts.AccessURL.String()

	if opts.Client == nil {
		opts.Client = http.DefaultClient
	}

	accessURL, err := opts.AccessURL.Parse("/healthz")
	if err != nil {
		r.Error = health.Errorf(health.CodeAccessURLInvalid, "parse healthz endpoint: %s", err)
		r.Severity = health.SeverityError
		return
	}

	req, err := http.NewRequestWithContext(ctx, "GET", accessURL.String(), nil)
	if err != nil {
		r.Error = health.Errorf(health.CodeAccessURLFetch, "create healthz request: %s", err)
		r.Severity = health.SeverityError
		return
	}

	res, err := opts.Client.Do(req)
	if err != nil {
		r.Error = health.Errorf(health.CodeAccessURLFetch, "get healthz endpoint: %s", err)
		r.Severity = health.SeverityError
		return
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		r.Error = health.Errorf(health.CodeAccessURLFetch, "read healthz response: %s", err)
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
