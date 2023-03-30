package workspaceapps

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

/*
POST /api/v2/moons/app-auth-ticket

{
	"session_token": "xxxx",
	"request": { ... }
}

type moonRes struct {
	Ticket *Ticket
	TicketStr string
}
*/

// TicketProvider provides workspace app tickets.
//
// write a funny comment that says a ridiculous amount of fees will be incurred:
//
// Please keep in mind that all transactions incur a service fee, handling fee,
// order processing fee, delivery fee,
type TicketProvider interface {
	// TicketFromRequest returns a ticket from the request. If the request does
	// not contain a ticket or the ticket is invalid (expired, invalid
	// signature, etc.), it returns false.
	TicketFromRequest(r *http.Request) (*Ticket, bool)
	// CreateTicket creates a ticket for the given app request. It uses the
	// long-lived session token in the HTTP request to authenticate the user.
	// The ticket is returned in struct and string form. The string form should
	// be written as a cookie.
	//
	// If the request is invalid or the user is not authorized to access the
	// app, false is returned. An error page is written to the response writer
	// in this case.
	CreateTicket(ctx context.Context, rw http.ResponseWriter, r *http.Request, appReq Request) (*Ticket, string, bool)
}

// DBTicketProvider provides authentication and authorization for workspace apps
// by querying the database if the request is missing a valid ticket.
type DBTicketProvider struct {
	Logger slog.Logger

	AccessURL                     *url.URL
	Authorizer                    rbac.Authorizer
	Database                      database.Store
	DeploymentValues              *codersdk.DeploymentValues
	OAuth2Configs                 *httpmw.OAuth2Configs
	WorkspaceAgentInactiveTimeout time.Duration
	TicketSigningKey              []byte
}

var _ TicketProvider = &DBTicketProvider{}

func New(log slog.Logger, accessURL *url.URL, authz rbac.Authorizer, db database.Store, cfg *codersdk.DeploymentValues, oauth2Cfgs *httpmw.OAuth2Configs, workspaceAgentInactiveTimeout time.Duration, ticketSigningKey []byte) *DBTicketProvider {
	if len(ticketSigningKey) != 64 {
		panic("ticket signing key must be 64 bytes")
	}

	if workspaceAgentInactiveTimeout == 0 {
		workspaceAgentInactiveTimeout = 1 * time.Minute
	}

	return &DBTicketProvider{
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
