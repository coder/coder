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

type Report struct {
	// Time is the time the report was generated at.
	Time time.Time `json:"time"`
	// Healthy is true if the report returns no errors.
	Healthy bool `json:"healthy"`

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
}

func Run(ctx context.Context, opts *ReportOptions) (*Report, error) {
	var report Report

	wg := &sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if err := recover(); err != nil {
				report.DERP.Error = xerrors.Errorf("%v", err)
			}
		}()

		report.DERP.Run(ctx, &DERPReportOptions{
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

		report.AccessURL.Run(ctx, &AccessURLOptions{
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
		report.Websocket.Run(ctx, &WebsocketReportOptions{
			APIKey:    opts.APIKey,
			AccessURL: opts.AccessURL,
		})
	}()

	wg.Wait()
	report.Time = time.Now()
	report.Healthy = report.DERP.Healthy &&
		report.AccessURL.Healthy &&
		report.Websocket.Healthy
	return &report, nil
}
