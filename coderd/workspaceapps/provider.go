package workspaceapps

import (
	"crypto/rsa"
	"net/url"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

// Provider provides authentication and authorization for workspace apps.
// TODO: also provide workspace apps as a whole to remove all app code from
// coderd.
type Provider struct {
	Logger slog.Logger

	AccessURL        *url.URL
	Authorizer       rbac.Authorizer
	Database         database.Store
	DeploymentConfig *codersdk.DeploymentConfig
	OAuth2Configs    *httpmw.OAuth2Configs
	TicketSigningKey *rsa.PrivateKey
}

func New(log slog.Logger, accessURL *url.URL, authz rbac.Authorizer, db database.Store, cfg *codersdk.DeploymentConfig, oauth2Cfgs *httpmw.OAuth2Configs, ticketSigningKey *rsa.PrivateKey) *Provider {
	return &Provider{
		Logger:           log,
		AccessURL:        accessURL,
		Authorizer:       authz,
		Database:         db,
		DeploymentConfig: cfg,
		OAuth2Configs:    oauth2Cfgs,
		TicketSigningKey: ticketSigningKey,
	}
}
