package healthcheck

import (
	"context"
	"net/url"
)

type WebsocketReport struct{}

func (*WebsocketReport) Run(ctx context.Context, accessURL *url.URL) {
	_, _ = ctx, accessURL
}
