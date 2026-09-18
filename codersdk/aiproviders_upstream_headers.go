package codersdk

import (
	"fmt"
	"strings"
)

// AIProviderSettingsTypeUpstreamHeaders is the _type discriminator value for
// AIProviderUpstreamHeadersSettings.
const AIProviderSettingsTypeUpstreamHeaders = "upstream-headers"

// AIProviderUpstreamHeadersSettingsVersion is the current schema version of
// AIProviderUpstreamHeadersSettings.
const AIProviderUpstreamHeadersSettingsVersion = 1

// ChatIDPlaceholder is the only template token allowed in an upstream header
// value. It resolves per request to the caller's conversation ID (the
// X-Coder-Chat-Id header sent by Coder Agents) so upstreams that key routing
// off a session header (e.g. OpenCode Zen's x-opencode-session) see a stable
// value per conversation. When the conversation ID is not reachable on the
// request, the gateway substitutes a deployment-stable UUID derived from the
// provider name instead. Literal values without the placeholder are sent
// verbatim on every request.
const ChatIDPlaceholder = "{{chat_id}}"

// AIProviderUpstreamHeadersSettings carries per-provider custom headers sent
// on every upstream request for that provider. Values are plain strings, not
// secrets: they are echoed back in GET and list responses so admins can audit
// them, and must never carry credentials (Authorization and X-Api-Key are
// rejected by validation because the key pool and BYOK machinery own those).
//
// A header value may embed ChatIDPlaceholder to get per-conversation
// stability; see the constant's doc for the resolution and fallback rules.
type AIProviderUpstreamHeadersSettings struct {
	// Headers maps header name to header value. Names are matched
	// case-insensitively per RFC 9110.
	Headers map[string]string `json:"headers,omitempty"`
}

func (AIProviderUpstreamHeadersSettings) settingsType() string {
	return AIProviderSettingsTypeUpstreamHeaders
}

func (AIProviderUpstreamHeadersSettings) settingsVersion() int {
	return AIProviderUpstreamHeadersSettingsVersion
}

// IsZero reports whether no custom headers are configured.
func (h AIProviderUpstreamHeadersSettings) IsZero() bool {
	return len(h.Headers) == 0
}

// MaxAIProviderUpstreamHeaders bounds the number of custom headers per
// provider so a misconfigured row cannot bloat every upstream request.
const MaxAIProviderUpstreamHeaders = 16

// MaxAIProviderUpstreamHeaderValueLen bounds a single header value so a
// misconfigured row cannot bloat every upstream request.
const MaxAIProviderUpstreamHeaderValueLen = 4096

// deniedAIProviderUpstreamHeaders names headers the custom-headers setting
// must not override. Credential headers are owned by the key pool and BYOK
// machinery; transport headers are owned by Go's HTTP stack and the gateway's
// proxy handling. Matching is case-insensitive.
var deniedAIProviderUpstreamHeaders = map[string]struct{}{
	"authorization":       {},
	"x-api-key":           {},
	"host":                {},
	"content-length":      {},
	"transfer-encoding":   {},
	"connection":          {},
	"upgrade":             {},
	"keep-alive":          {},
	"proxy-authenticate":  {},
	"proxy-authorization": {},
	"proxy-connection":    {},
	"te":                  {},
	"trailer":             {},
}

// validateAIProviderUpstreamHeaders checks a headers blob for structural
// validity: count and size bounds, RFC 9110 token names, non-empty values
// without CR/LF, no case-insensitive duplicates, no denied overrides, and no
// unknown {{...}} placeholders. An empty (zero) value is valid: on create it
// means "no custom headers"; on update it clears them.
func validateAIProviderUpstreamHeaders(h AIProviderUpstreamHeadersSettings) []ValidationError {
	var validations []ValidationError
	if len(h.Headers) > MaxAIProviderUpstreamHeaders {
		validations = append(validations, ValidationError{
			Field:  "settings.headers",
			Detail: fmt.Sprintf("at most %d custom headers are supported, got %d", MaxAIProviderUpstreamHeaders, len(h.Headers)),
		})
	}
	seen := make(map[string]string, len(h.Headers))
	for name, value := range h.Headers {
		field := fmt.Sprintf("settings.headers[%q]", name)
		lowered := strings.ToLower(name)
		if !isHTTPToken(name) {
			validations = append(validations, ValidationError{
				Field:  field,
				Detail: "header name must be a valid HTTP token (RFC 9110 section 5.1)",
			})
			continue
		}
		if _, denied := deniedAIProviderUpstreamHeaders[lowered]; denied {
			validations = append(validations, ValidationError{
				Field:  field,
				Detail: fmt.Sprintf("header %q is managed by the gateway and cannot be overridden", name),
			})
			continue
		}
		if prev, ok := seen[lowered]; ok {
			validations = append(validations, ValidationError{
				Field:  field,
				Detail: fmt.Sprintf("duplicate header %q (already set as %q); names are case-insensitive", name, prev),
			})
			continue
		}
		seen[lowered] = name
		if value == "" {
			validations = append(validations, ValidationError{
				Field:  field,
				Detail: "header value must not be empty",
			})
			continue
		}
		if len(value) > MaxAIProviderUpstreamHeaderValueLen {
			validations = append(validations, ValidationError{
				Field:  field,
				Detail: fmt.Sprintf("header value must be at most %d bytes, got %d", MaxAIProviderUpstreamHeaderValueLen, len(value)),
			})
		}
		if strings.ContainsAny(value, "\r\n") {
			validations = append(validations, ValidationError{
				Field:  field,
				Detail: "header value must not contain CR or LF",
			})
		}
		validations = append(validations, validateAIProviderUpstreamHeaderPlaceholders(field, value)...)
	}
	return validations
}

// validateAIProviderUpstreamHeaderPlaceholders rejects unknown {{...}}
// template tokens in a header value so a typo fails fast at write time
// instead of reaching the upstream verbatim. ChatIDPlaceholder is the only
// token the gateway resolves.
func validateAIProviderUpstreamHeaderPlaceholders(field, value string) []ValidationError {
	var validations []ValidationError
	rest := value
	for {
		start := strings.Index(rest, "{{")
		if start < 0 {
			return validations
		}
		end := strings.Index(rest[start:], "}}")
		if end < 0 {
			validations = append(validations, ValidationError{
				Field:  field,
				Detail: "header value has an unterminated {{ placeholder",
			})
			return validations
		}
		token := rest[start : start+end+2]
		if token != ChatIDPlaceholder {
			validations = append(validations, ValidationError{
				Field:  field,
				Detail: fmt.Sprintf("unknown placeholder %q; only %q is supported", token, ChatIDPlaceholder),
			})
		}
		rest = rest[start+end+2:]
	}
}

// isHTTPToken reports whether s is a valid RFC 9110 token (the production
// for field names). Empty strings are not tokens.
func isHTTPToken(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x21 || c > 0x7e {
			return false
		}
		switch c {
		case '(', ')', '<', '>', '@', ',', ';', ':', '\\', '"', '/', '[', ']', '?', '=', '{', '}':
			return false
		}
	}
	return true
}
