package fositeprovider

import (
	"log"
	"net/http"
)

// TODO: Not sure how TokenEndpoint works with sessions.
func (p *Provider) TokenEndpoint(rw http.ResponseWriter, req *http.Request) {
	// This context will be passed to all methods.
	ctx := req.Context()

	// Create an empty session object which will be passed to the request handlers
	// TODO: Why do we need an empty session here?
	mySessionData := p.EmptySession()

	// This will create an access request object and iterate through the registered TokenEndpointHandlers to validate the request.
	accessRequest, err := p.provider.NewAccessRequest(ctx, req, mySessionData)

	// Catch any errors, e.g.:
	// * unknown client
	// * invalid redirect
	// * ...
	if err != nil {
		log.Printf("Error occurred in NewAccessRequest: %+v", err)
		p.provider.WriteAccessError(ctx, rw, accessRequest, err)
		return
	}

	// If this is a client_credentials grant, grant all requested scopes
	// NewAccessRequest validated that all requested scopes the client is allowed to perform
	// based on configured scope matching strategy.
	if accessRequest.GetGrantTypes().ExactOne("client_credentials") {
		for _, scope := range accessRequest.GetRequestedScopes() {
			accessRequest.GrantScope(scope)
		}
	}

	// Next we create a response for the access request. Again, we iterate through the TokenEndpointHandlers
	// and aggregate the result in response.
	response, err := p.provider.NewAccessResponse(ctx, accessRequest)
	if err != nil {
		log.Printf("Error occurred in NewAccessResponse: %+v", err)
		p.provider.WriteAccessError(ctx, rw, accessRequest, err)
		return
	}

	// All done, send the response.
	p.provider.WriteAccessResponse(ctx, rw, accessRequest, response)

	// The client now has a valid access token
}
