package nats

import "golang.org/x/xerrors"

// Subject is a validated NATS publish subject in the coder.v1 namespace.
type Subject string

// LegacyEventSubject maps a legacy coderd/database/pubsub event name (for
// example "workspace_owner:<uuid>") to a NATS subject under
// DefaultSubjectPrefix.
func LegacyEventSubject(event string) (Subject, error) {
	_ = event
	// TODO: implement legacy event to subject mapping.
	return "", xerrors.New("not implemented")
}

// BuildSubject builds a native coder.v1 subject from a domain and tokens.
func BuildSubject(domain string, tokens ...string) (Subject, error) {
	_ = domain
	_ = tokens
	// TODO: implement subject construction.
	return "", xerrors.New("not implemented")
}

// ValidateSubject validates a full publish subject. It rejects wildcard
// tokens by design.
func ValidateSubject(subject string) error {
	_ = subject
	// TODO: implement subject validation.
	return xerrors.New("not implemented")
}

// ValidateToken validates a single subject token.
func ValidateToken(token string) error {
	_ = token
	// TODO: implement token validation.
	return xerrors.New("not implemented")
}
