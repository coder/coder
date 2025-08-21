package fositestorage

import (
	"github.com/ory/fosite"

	"github.com/coder/coder/v2/coderd/database"
)

var _ fosite.Client = (*Client)(nil)

// TODO: We can implement more client interfaces if needed.
//var _ fosite.ClientWithSecretRotation = (*Client)(nil)
//var _ fosite.OpenIDConnectClient = (*Client)(nil)
//var _ fosite.ResponseModeClient = (*Client)(nil)

// Client
// See fosite.DefaultClient for default implementation of most methods.
type Client struct {
	App     database.OAuth2ProviderApp
	Secrets []database.OAuth2ProviderAppSecret
	_       fosite.DefaultClient
}

func (c Client) GetID() string {
	return c.App.ID.String()
}

func (c Client) GetHashedSecret() []byte {
	// TODO: Why do we have more than one secret?
	return c.Secrets[0].HashedSecret
}

func (c Client) GetRedirectURIs() []string {
	return c.App.RedirectUris
}

func (c Client) GetGrantTypes() fosite.Arguments {
	return c.App.GrantTypes
}

func (c Client) GetResponseTypes() fosite.Arguments {
	return c.App.ResponseTypes
}

func (c Client) GetScopes() fosite.Arguments {
	return []string{}
}

func (c Client) IsPublic() bool {
	return false // Is this right?
}

func (c Client) GetAudience() fosite.Arguments {
	return []string{"https://coder.com"} // TODO: should be access url
}
