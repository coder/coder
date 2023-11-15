package healthcheck

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/healthcheck/derphealth"
	"github.com/coder/coder/v2/coderd/util/ptr"
)

const (
	SectionDERP      string = "DERP"
	SectionAccessURL string = "AccessURL"
	SectionWebsocket string = "Websocket"
	SectionDatabase  string = "Database"
)

type Checker interface {
	DERP(ctx context.Context, opts *derphealth.ReportOptions) derphealth.Report
	AccessURL(ctx context.Context, opts *AccessURLReportOptions) AccessURLReport
	Websocket(ctx context.Context, opts *WebsocketReportOptions) WebsocketReport
	Database(ctx context.Context, opts *DatabaseReportOptions) DatabaseReport
}

// @typescript-generate Report
type Report struct {
	// Time is the time the report was generated at.
	Time time.Time `json:"time"`
	// Healthy is true if the report returns no errors.
	Healthy bool `json:"healthy"`
	// FailingSections is a list of sections that have failed their healthcheck.
	FailingSections []string `json:"failing_sections"`

	DERP      derphealth.Report `json:"derp"`
	AccessURL AccessURLReport   `json:"access_url"`
	Websocket WebsocketReport   `json:"websocket"`
	Database  DatabaseReport    `json:"database"`

	// The Coder version of the server that the report was generated on.
	CoderVersion string `json:"coder_version"`
}

type ReportOptions struct {
	AccessURL  AccessURLReportOptions
	Database   DatabaseReportOptions
	DerpHealth derphealth.ReportOptions
	Websocket  WebsocketReportOptions

	Checker Checker
}

type defaultChecker struct{}

func (defaultChecker) DERP(ctx context.Context, opts *derphealth.ReportOptions) (report derphealth.Report) {
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
				report.DERP.Error = ptr.Ref(fmt.Sprint(err))
			}
		}()

		report.DERP = opts.Checker.DERP(ctx, &opts.DerpHealth)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if err := recover(); err != nil {
				report.AccessURL.Error = ptr.Ref(fmt.Sprint(err))
			}
		}()

		report.AccessURL = opts.Checker.AccessURL(ctx, &opts.AccessURL)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if err := recover(); err != nil {
				report.Websocket.Error = ptr.Ref(fmt.Sprint(err))
			}
		}()

		report.Websocket = opts.Checker.Websocket(ctx, &opts.Websocket)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if err := recover(); err != nil {
				report.Database.Error = ptr.Ref(fmt.Sprint(err))
			}
		}()

		report.Database = opts.Checker.Database(ctx, &opts.Database)
	}()

	report.CoderVersion = buildinfo.Version()
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

func convertError(err error) *string {
	if err != nil {
		return ptr.Ref(err.Error())
	}

	return nil
}
