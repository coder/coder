package healthcheck

import (
	"context"
	"time"

	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"
)

type Report struct {
	Time time.Time  `json:"time"`
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
	return &report, nil
}
