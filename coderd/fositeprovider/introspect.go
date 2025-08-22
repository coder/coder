package fositeprovider

import (
	"net/http"

	"cdr.dev/slog"
)

func (p *Provider) IntrospectionEndpoint(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	mySessionData := p.EmptySession()
	ir, err := p.provider.NewIntrospectionRequest(ctx, req, mySessionData)
	if err != nil {
		p.logger.Error(ctx, "error occurred in NewIntrospectionRequest", slog.Error(err))
		p.provider.WriteIntrospectionError(ctx, rw, err)
		return
	}

	p.provider.WriteIntrospectionResponse(ctx, rw, ir)
}
