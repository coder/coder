package scim

import (
	"github.com/elimity-com/scim"
	"github.com/elimity-com/scim/optional"
	"github.com/elimity-com/scim/schema"

	"github.com/coder/coder/v2/coderd/database"
)

type Server struct {
	opts *Options
	*scim.Server
}

type Options struct {
	DB database.Store
}

func New(opts *Options) *Server {
	args := &scim.ServerArgs{
		ServiceProviderConfig: &scim.ServiceProviderConfig{
			DocumentationURI: optional.NewString("https://coder.com/docs/admin/users/oidc-auth#scim"),
			AuthenticationSchemes: []scim.AuthenticationScheme{
				{
					Type:        scim.AuthenticationTypeOauthBearerToken,
					Name:        "HTTP Header Authentication",
					Description: "Authentication scheme using the Authorization header with the shared token",
					// TODO: Add docs for this and link here
					SpecURI:          optional.String{},
					DocumentationURI: optional.String{},
					Primary:          true,
				},
			},
			MaxResults:       0,
			SupportFiltering: false,
			SupportPatch:     false,
		},
		ResourceTypes: []scim.ResourceType{
			{
				ID:               optional.NewString("User"),
				Name:             "User",
				Description:      optional.String{},
				Endpoint:         "/User",
				Schema:           schema.CoreUserSchema(),
				SchemaExtensions: nil,
				Handler:          nil,
			},
		},
	}

	srv, err := scim.NewServer(args)
	if err != nil {
		return nil
	}

	return &Server{
		Server: &srv,
	}
}
