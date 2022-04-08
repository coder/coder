// Package session provides session authentication via middleware for the Coder
// HTTP API. It also exposes the Actor type, which is the intermediary layer
// between identity and RBAC authorization.
//
// The Actor types exposed by this package are consumed by the authz packages to
// determine if a request is authorized to perform an API action.
package session
