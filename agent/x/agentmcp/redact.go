package agentmcp

import (
	"net/url"
	"sort"
	"strings"
)

// redactedPlaceholder replaces configured secret material in
// user-facing MCP diagnostics.
const redactedPlaceholder = "[redacted]"

// minRedactLength skips env and header values too short to be secrets.
// Redacting one- or two-character values (DEBUG=1) would mangle
// addresses and exit codes in the surrounding error text.
const minRedactLength = 4

// sanitizeMCPError renders err for the discovery report with every
// configured secret removed: URL userinfo and query strings, every env
// value, and every header value. Transport errors echo the URL and
// subprocess errors can echo the environment, so the raw text is never
// safe to publish to chats.
func sanitizeMCPError(cfg ServerConfig, err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	secrets := make([]string, 0, len(cfg.Env)+len(cfg.Headers)+2)
	for _, v := range cfg.Env {
		if len(v) >= minRedactLength {
			secrets = append(secrets, v)
		}
	}
	for _, v := range cfg.Headers {
		if len(v) >= minRedactLength {
			secrets = append(secrets, v)
		}
	}
	if cfg.URL != "" {
		if u, parseErr := url.Parse(cfg.URL); parseErr == nil {
			if u.User != nil && u.User.String() != "" {
				secrets = append(secrets, u.User.String())
				if pw, ok := u.User.Password(); ok && pw != "" {
					secrets = append(secrets, pw)
				}
			}
			if u.RawQuery != "" {
				secrets = append(secrets, u.RawQuery)
				for _, values := range u.Query() {
					for _, v := range values {
						if len(v) >= minRedactLength {
							secrets = append(secrets, v)
						}
					}
				}
			}
		}
	}
	// Longest first so a secret that contains another is replaced whole.
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, s := range secrets {
		msg = strings.ReplaceAll(msg, s, redactedPlaceholder)
	}
	return msg
}
