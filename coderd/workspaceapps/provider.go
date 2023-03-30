package workspaceapps

import (
	"net/url"
	"time"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

// Provider provides authentication and authorization for workspace apps.
// TODO(@deansheather): also provide workspace apps as a whole to remove all app
// code from coderd.
type Provider struct {
	Logger slog.Logger

	AccessURL                     *url.URL
	Authorizer                    rbac.Authorizer
	Database                      database.Store
	DeploymentValues              *codersdk.DeploymentValues
	OAuth2Configs                 *httpmw.OAuth2Configs
	WorkspaceAgentInactiveTimeout time.Duration
	TicketSigningKey              []byte
}

func New(log slog.Logger, accessURL *url.URL, authz rbac.Authorizer, db database.Store, cfg *codersdk.DeploymentValues, oauth2Cfgs *httpmw.OAuth2Configs, workspaceAgentInactiveTimeout time.Duration, ticketSigningKey []byte) *Provider {
	if len(ticketSigningKey) != 64 {
		panic("ticket signing key must be 64 bytes")
	}

	if workspaceAgentInactiveTimeout == 0 {
		workspaceAgentInactiveTimeout = 1 * time.Minute
	}

	return &Provider{
		Logger:                        log,
		AccessURL:                     accessURL,
		Authorizer:                    authz,
		Database:                      db,
		DeploymentValues:              cfg,
		OAuth2Configs:                 oauth2Cfgs,
		WorkspaceAgentInactiveTimeout: workspaceAgentInactiveTimeout,
		TicketSigningKey:              ticketSigningKey,
	}
}
