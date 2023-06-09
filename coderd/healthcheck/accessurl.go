package healthcheck

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/xerrors"
)

type AccessURLReport struct {
	Healthy         bool   `json:"healthy"`
	Reachable       bool   `json:"reachable"`
	StatusCode      int    `json:"status_code"`
	HealthzResponse string `json:"healthz_response"`
	Error           error  `json:"error"`
}

type AccessURLOptions struct {
	AccessURL *url.URL
	Client    *http.Client
}

func (r *AccessURLReport) Run(ctx context.Context, opts *AccessURLOptions) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if opts.AccessURL == nil {
		r.Error = xerrors.New("access URL is nil")
		return
	}

	if opts.Client == nil {
		opts.Client = http.DefaultClient
	}

	accessURL, err := opts.AccessURL.Parse("/healthz")
	if err != nil {
		r.Error = xerrors.Errorf("parse healthz endpoint: %w", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, "GET", accessURL.String(), nil)
	if err != nil {
		r.Error = xerrors.Errorf("create healthz request: %w", err)
		return
	}

	res, err := opts.Client.Do(req)
	if err != nil {
		r.Error = xerrors.Errorf("get healthz endpoint: %w", err)
		return
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		r.Error = xerrors.Errorf("read healthz response: %w", err)
		return
	}

	r.Reachable = true
	r.Healthy = res.StatusCode == http.StatusOK
	r.StatusCode = res.StatusCode
	r.HealthzResponse = string(body)
}
