package nats

import (
	"strings"

	"golang.org/x/xerrors"
)

// Subject is a validated NATS publish subject in the coder.v1 namespace.
type Subject string

// Error sentinels for subject validation. Callers can match these via
// errors.Is to distinguish failure modes.
var (
	// ErrEmptySubject is returned when a subject or required component is
	// empty.
	ErrEmptySubject = xerrors.New("nats: subject is empty")

	// ErrInvalidSubject is returned when a subject does not satisfy the
	// coder.v1 publish subject rules.
	ErrInvalidSubject = xerrors.New("nats: invalid subject")

	// ErrInvalidToken is returned when a single subject token contains
	// disallowed characters or is otherwise malformed.
	ErrInvalidToken = xerrors.New("nats: invalid subject token")
)

// LegacyEventSubject maps a legacy coderd/database/pubsub event name (for
// example "workspace_owner:<uuid>") to a NATS subject under
// DefaultSubjectPrefix. Colons in the legacy event are translated to dots so
// each colon-separated component becomes its own subject token.
func LegacyEventSubject(event string) (Subject, error) {
	if event == "" {
		return "", xerrors.Errorf("legacy event: %w", ErrEmptySubject)
	}
	const mid = ".pubsub."
	var b strings.Builder
	b.Grow(len(DefaultSubjectPrefix) + len(mid) + len(event))
	_, _ = b.WriteString(DefaultSubjectPrefix)
	_, _ = b.WriteString(mid)
	// tokenStart tracks the index in event where the current token began,
	// so we can report an empty-token error with the same wrapping as the
	// previous Split + ValidateToken implementation.
	tokenStart := 0
	for i := 0; i < len(event); i++ {
		c := event[i]
		if c == ':' {
			if i == tokenStart {
				return "", xerrors.Errorf("legacy event %q: empty token: %w", event, ErrInvalidToken)
			}
			_ = b.WriteByte('.')
			tokenStart = i + 1
			continue
		}
		switch {
		case c >= 'A' && c <= 'Z':
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '_' || c == '-':
		default:
			return "", xerrors.Errorf("legacy event %q: token contains disallowed character %q: %w", event, rune(c), ErrInvalidToken)
		}
		_ = b.WriteByte(c)
	}
	if tokenStart == len(event) {
		// Trailing colon (or all-colons): the last token is empty.
		return "", xerrors.Errorf("legacy event %q: empty token: %w", event, ErrInvalidToken)
	}
	return Subject(b.String()), nil
}

// BuildSubject builds a native coder.v1 subject from a domain and tokens.
// The domain and every token must satisfy ValidateToken. At least one token
// is required after the domain.
func BuildSubject(domain string, tokens ...string) (Subject, error) {
	if err := ValidateToken(domain); err != nil {
		return "", xerrors.Errorf("domain: %w", err)
	}
	if len(tokens) == 0 {
		return "", xerrors.Errorf("build subject %q: %w", domain, ErrEmptySubject)
	}
	for i, t := range tokens {
		if err := ValidateToken(t); err != nil {
			return "", xerrors.Errorf("token[%d]: %w", i, err)
		}
	}
	parts := make([]string, 0, 1+len(tokens))
	parts = append(parts, domain)
	parts = append(parts, tokens...)
	return Subject(DefaultSubjectPrefix + "." + strings.Join(parts, ".")), nil
}

// ValidateSubject validates a fully-formed publish subject. It must begin
// with DefaultSubjectPrefix, contain at least one further token, and every
// dot-separated token must satisfy ValidateToken. Wildcards are rejected.
func ValidateSubject(subject string) error {
	if subject == "" {
		return xerrors.Errorf("validate subject: %w", ErrEmptySubject)
	}
	prefix := DefaultSubjectPrefix + "."
	if !strings.HasPrefix(subject, prefix) {
		return xerrors.Errorf("subject %q missing prefix %q: %w", subject, prefix, ErrInvalidSubject)
	}
	rest := subject[len(prefix):]
	if rest == "" {
		return xerrors.Errorf("subject %q has no tokens after prefix: %w", subject, ErrInvalidSubject)
	}
	tokens := strings.Split(rest, ".")
	for _, t := range tokens {
		if err := ValidateToken(t); err != nil {
			return xerrors.Errorf("subject %q: %w", subject, err)
		}
	}
	return nil
}

// ValidateToken validates a single subject token. Allowed characters are
// ASCII letters, digits, underscore, and hyphen. Empty tokens, whitespace,
// wildcards (*, >), and any non-ASCII rune are rejected.
func ValidateToken(token string) error {
	if token == "" {
		return xerrors.Errorf("empty token: %w", ErrInvalidToken)
	}
	for _, r := range token {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return xerrors.Errorf("token %q contains disallowed character %q: %w", token, r, ErrInvalidToken)
		}
	}
	return nil
}
