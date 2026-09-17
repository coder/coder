package agentmcp

import (
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"
)

// redactedPlaceholder replaces configured secret material in
// user-facing MCP diagnostics.
const redactedPlaceholder = "[redacted]"

// minRedactLength skips env and header values too short to be secrets.
// Redacting one- or two-character values (DEBUG=1) would mangle
// addresses and exit codes in the surrounding error text.
const minRedactLength = 4

func envValues(env []string) []string {
	var values []string
	for _, kv := range env {
		if _, v, ok := strings.Cut(kv, "="); ok && len(v) >= minRedactLength {
			values = append(values, v)
		}
	}
	return values
}

// maxServerNameBytes matches coderd's per-resource source cap: the
// server name is the source of every mcp_server resource, and a longer
// one would make coderd reject the whole context push.
const maxServerNameBytes = 1024

// maxDiagnosticBytes matches coderd's per-resource error cap. A longer
// error makes coderd reject the whole context push, and the agent
// retries that push indefinitely.
const maxDiagnosticBytes = 4096

const truncatedSuffix = "... [truncated]"

// boundDiagnostic truncates msg to maxDiagnosticBytes on a rune
// boundary.
func boundDiagnostic(msg string) string {
	if len(msg) <= maxDiagnosticBytes {
		return msg
	}
	cut := maxDiagnosticBytes - len(truncatedSuffix)
	for cut > 0 && !utf8.RuneStart(msg[cut]) {
		cut--
	}
	return msg[:cut] + truncatedSuffix
}

// sanitizeMCPError renders err for the discovery report with every
// configured secret removed: URL userinfo, path, and query string,
// every env value, every header value, every stdio argument that is
// not itself a flag, and every inherited value (secrets the agent
// injects into the server environment). Transport errors echo the URL
// and a server can echo its command line and environment in a
// JSON-RPC error, so the raw text is never safe to publish to chats.
// The result is bounded to maxDiagnosticBytes.
func sanitizeMCPError(cfg ServerConfig, inherited []string, err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	secrets := make([]string, 0, len(cfg.Env)+len(cfg.Headers)+len(cfg.Args)+len(inherited)+2)
	for _, v := range cfg.Env {
		if len(v) >= minRedactLength {
			secrets = append(secrets, v)
		}
	}
	for _, v := range inherited {
		if len(v) >= minRedactLength {
			secrets = append(secrets, v)
		}
	}
	// Credentials are commonly passed as "--token X" or "--token=X".
	// Flags are public; their values and every positional arg are not.
	for _, a := range cfg.Args {
		if strings.HasPrefix(a, "-") {
			if _, v, ok := strings.Cut(a, "="); ok && len(v) >= minRedactLength {
				secrets = append(secrets, v)
			}
			continue
		}
		if len(a) >= minRedactLength {
			secrets = append(secrets, a)
		}
	}
	// A server may echo only the credential part of a scheme-prefixed
	// header ("Bearer x" reported as "invalid token x"), so that part is
	// registered on its own as well.
	for _, v := range cfg.Headers {
		if len(v) >= minRedactLength {
			secrets = append(secrets, v)
		}
		if _, credential, ok := strings.Cut(v, " "); ok {
			if credential = strings.TrimSpace(credential); len(credential) >= minRedactLength {
				secrets = append(secrets, credential)
			}
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
			// Hosted MCP endpoints commonly carry the credential as a
			// path segment, so the whole path goes; scheme, host, and
			// the leading slash stay readable.
			for _, p := range []string{u.EscapedPath(), u.Path} {
				if p = strings.TrimPrefix(p, "/"); len(p) >= minRedactLength {
					secrets = append(secrets, p)
				}
			}
		}
	}
	// Longest first so a secret that contains another is replaced whole.
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, s := range secrets {
		msg = strings.ReplaceAll(msg, s, redactedPlaceholder)
	}
	return boundDiagnostic(msg)
}
