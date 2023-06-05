package healthcheck

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"
)

const (
	SectionDERP      string = "DERP"
	SectionAccessURL string = "AccessURL"
	SectionWebsocket string = "Websocket"
)

type Checker interface {
	DERP(ctx context.Context, opts *DERPReportOptions) DERPReport
	AccessURL(ctx context.Context, opts *AccessURLOptions) AccessURLReport
	Websocket(ctx context.Context, opts *WebsocketReportOptions) WebsocketReport
}

type Report struct {
	// Time is the time the report was generated at.
	Time time.Time `json:"time"`
	// Healthy is true if the report returns no errors.
	Healthy         bool     `json:"healthy"`
	FailingSections []string `json:"failing_sections"`

	DERP      DERPReport      `json:"derp"`
	AccessURL AccessURLReport `json:"access_url"`
	Websocket WebsocketReport `json:"websocket"`
}

type ReportOptions struct {
	// TODO: support getting this over HTTP?
	DERPMap   *tailcfg.DERPMap
	AccessURL *url.URL
	Client    *http.Client
	APIKey    string

	Checker Checker
}

type defaultChecker struct{}

func (defaultChecker) DERP(ctx context.Context, opts *DERPReportOptions) (report DERPReport) {
	report.Run(ctx, opts)
	return report
}

func (defaultChecker) AccessURL(ctx context.Context, opts *AccessURLOptions) (report AccessURLReport) {
	report.Run(ctx, opts)
	return report
}

func (defaultChecker) Websocket(ctx context.Context, opts *WebsocketReportOptions) (report WebsocketReport) {
	report.Run(ctx, opts)
	return report
}

func Run(ctx context.Context, opts *ReportOptions) *Report {
	var (
		wg     sync.WaitGroup
		report Report
	)

	if opts.Checker == nil {
		opts.Checker = defaultChecker{}
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if err := recover(); err != nil {
				report.DERP.Error = xerrors.Errorf("%v", err)
			}
		}()

		report.DERP = opts.Checker.DERP(ctx, &DERPReportOptions{
			DERPMap: opts.DERPMap,
		})
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if err := recover(); err != nil {
				report.AccessURL.Error = xerrors.Errorf("%v", err)
			}
		}()

		report.AccessURL = opts.Checker.AccessURL(ctx, &AccessURLOptions{
			AccessURL: opts.AccessURL,
			Client:    opts.Client,
		})
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if err := recover(); err != nil {
				report.Websocket.Error = xerrors.Errorf("%v", err)
			}
		}()

		report.Websocket = opts.Checker.Websocket(ctx, &WebsocketReportOptions{
			APIKey:    opts.APIKey,
			AccessURL: opts.AccessURL,
		})
	}()

	wg.Wait()
	report.Time = time.Now()
	if !report.DERP.Healthy {
		report.FailingSections = append(report.FailingSections, SectionDERP)
	}
	if !report.AccessURL.Healthy {
		report.FailingSections = append(report.FailingSections, SectionAccessURL)
	}
	if !report.Websocket.Healthy {
		report.FailingSections = append(report.FailingSections, SectionWebsocket)
	}

	report.Healthy = len(report.FailingSections) == 0
	return &report
}
