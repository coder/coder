package healthcheck

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	"github.com/coder/coder/coderd/database"
)

const (
	SectionDERP      string = "DERP"
	SectionAccessURL string = "AccessURL"
	SectionWebsocket string = "Websocket"
	SectionDatabase  string = "Database"
)

type Checker interface {
	DERP(ctx context.Context, opts *DERPReportOptions) DERPReport
	AccessURL(ctx context.Context, opts *AccessURLReportOptions) AccessURLReport
	Websocket(ctx context.Context, opts *WebsocketReportOptions) WebsocketReport
	Database(ctx context.Context, opts *DatabaseReportOptions) DatabaseReport
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
	Database  DatabaseReport  `json:"database"`
}

type ReportOptions struct {
	DB database.Store
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

func (defaultChecker) AccessURL(ctx context.Context, opts *AccessURLReportOptions) (report AccessURLReport) {
	report.Run(ctx, opts)
	return report
}

func (defaultChecker) Websocket(ctx context.Context, opts *WebsocketReportOptions) (report WebsocketReport) {
	report.Run(ctx, opts)
	return report
}

func (defaultChecker) Database(ctx context.Context, opts *DatabaseReportOptions) (report DatabaseReport) {
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

		report.AccessURL = opts.Checker.AccessURL(ctx, &AccessURLReportOptions{
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

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if err := recover(); err != nil {
				report.Database.Error = xerrors.Errorf("%v", err)
			}
		}()

		report.Database = opts.Checker.Database(ctx, &DatabaseReportOptions{
			DB: opts.DB,
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
	if !report.Database.Healthy {
		report.FailingSections = append(report.FailingSections, SectionDatabase)
	}

	report.Healthy = len(report.FailingSections) == 0
	return &report
}
