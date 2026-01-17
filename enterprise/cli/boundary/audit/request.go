//nolint:revive,gocritic,errname,unconvert
package audit

type Auditor interface {
	AuditRequest(req Request)
}

// Request represents information about an HTTP request for auditing
type Request struct {
	Method  string
	URL     string // The fully qualified request URL (scheme, domain, optional path).
	Host    string
	Allowed bool
	Rule    string // The rule that matched (if any)
}
