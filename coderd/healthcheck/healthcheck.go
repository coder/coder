package healthcheck

import (
	"context"
	"time"

	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"
)

type Report struct {
	// Time is the time the report was generated at.
	Time time.Time `json:"time"`
	// Healthy is true if the report returns no errors.
	Healthy bool `json:"pass"`

	DERP DERPReport `json:"derp"`

	// TODO
	// AccessURL AccessURLReport
	// Websocket WebsocketReport
}

type ReportOptions struct {
	// TODO: support getting this over HTTP?
	DERPMap *tailcfg.DERPMap
}

func Run(ctx context.Context, opts *ReportOptions) (*Report, error) {
	var report Report

	err := report.DERP.Run(ctx, &DERPReportOptions{
		DERPMap: opts.DERPMap,
	})
	if err != nil {
		return nil, xerrors.Errorf("run derp: %w", err)
	}

	report.Time = time.Now()
	report.Healthy = report.DERP.Healthy
	return &report, nil
}
