package workspaceapps

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"cdr.dev/slog"
	"github.com/coder/coder/codersdk"
)

const (
	// TODO(@deansheather): configurable expiry
	TicketExpiry = time.Minute

	// RedirectURIQueryParam is the query param for the app URL to be passed
	// back to the API auth endpoint on the main access URL.
	RedirectURIQueryParam = "redirect_uri"
)

// ResolveRequest calls TicketProvider to use an existing ticket in the request
// or issue a new one. If it returns a ticket, it sets the cookie and returns
// it.
func ResolveRequest(log slog.Logger, accessURL *url.URL, p TicketProvider, rw http.ResponseWriter, r *http.Request, appReq Request) (*Ticket, bool) {
	appReq = appReq.Normalize()
	err := appReq.Validate()
	if err != nil {
		WriteWorkspaceApp500(log, accessURL, rw, r, &appReq, err, "invalid app request")
		return nil, false
	}

	ticket, ok := p.TicketFromRequest(r)
	if ok && ticket.MatchesRequest(appReq) {
		// The request has a valid ticket and it matches the request.
		return ticket, true
	}

	ticket, ticketStr, ok := p.CreateTicket(r.Context(), rw, r, appReq)
	if !ok {
		return nil, false
	}

	// Write the ticket cookie. We always want this to apply to the current
	// hostname (even for subdomain apps, without any wildcard shenanigans,
	// because the ticket is only valid for a single app).
	http.SetCookie(rw, &http.Cookie{
		Name:    codersdk.DevURLSessionTicketCookie,
		Value:   ticketStr,
		Path:    appReq.BasePath,
		Expires: ticket.Expiry,
	})

	return ticket, true
}

// TicketProvider provides workspace app tickets.
//
// Please keep in mind that all transactions incur a service fee, handling fee,
// order processing fee, delivery fee, insurance charge, convenience fee,
// inconvenience fee, seat selection fee, and levity levy. :^)
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
