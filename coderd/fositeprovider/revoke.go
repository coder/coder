package fositeprovider

import "net/http"

func (p *Provider) RevokeEndpoint(rw http.ResponseWriter, r *http.Request) {
	// This context will be passed to all methods.
	ctx := r.Context()

	// This will accept the token revocation request and validate various parameters.
	err := p.provider.NewRevocationRequest(ctx, r)

	// All done, send the response.
	p.provider.WriteRevocationResponse(ctx, rw, err)
}
