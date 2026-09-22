package coderd

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/net/http/httpguts"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/safedial"
)

var inlineMCPServerSlugPattern = regexp.MustCompile(fmt.Sprintf(`^[A-Za-z0-9][A-Za-z0-9_-]{0,%d}$`, codersdk.MaxInlineMCPServerSlugBytes-1))

func writeChatCallerSuppliedToolsDisabled(ctx context.Context, rw http.ResponseWriter) {
	httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
		Message: "Caller-supplied tools are disabled on this deployment.",
		Detail:  "The server runs with --disable-chat-caller-supplied-tools. Remove unsafe_dynamic_tools and inline_mcp_servers from the request.",
	})
}

func writeInlineMCPServersExperimentRequired(ctx context.Context, rw http.ResponseWriter) {
	httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
		Message: "Inline MCP servers are not enabled on this deployment.",
		Detail:  fmt.Sprintf("Enable the %s experiment.", codersdk.ExperimentChatInlineMCPServers),
	})
}

func writeInlineMCPServersInvalid(ctx context.Context, rw http.ResponseWriter, validations []codersdk.ValidationError) {
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message:     "Invalid inline_mcp_servers.",
		Validations: validations,
	})
}

func validateInlineMCPServers(
	servers []codersdk.InlineMCPServerRequest,
	allowedIPRanges []netip.Prefix,
) []codersdk.ValidationError {
	if len(servers) > codersdk.MaxInlineMCPServers {
		return []codersdk.ValidationError{{
			Field:  "inline_mcp_servers",
			Detail: fmt.Sprintf("must contain at most %d servers", codersdk.MaxInlineMCPServers),
		}}
	}

	if inlineMCPServersSize(servers) > codersdk.MaxInlineMCPServersBytes {
		return []codersdk.ValidationError{{
			Field: "inline_mcp_servers",
			Detail: fmt.Sprintf(
				"total size must not exceed %d bytes",
				codersdk.MaxInlineMCPServersBytes,
			),
		}}
	}

	seenSlugs := make(map[string]struct{}, len(servers))
	var validations []codersdk.ValidationError
	for i, server := range servers {
		slugField := fmt.Sprintf("inline_mcp_servers[%d].slug", i)
		if !inlineMCPServerSlugPattern.MatchString(server.Slug) {
			validations = append(validations, codersdk.ValidationError{
				Field: slugField,
				Detail: fmt.Sprintf(
					"must be 1 to %d ASCII letters, numbers, underscores, or hyphens, and must start with a letter or number",
					codersdk.MaxInlineMCPServerSlugBytes,
				),
			})
		} else if _, ok := seenSlugs[server.Slug]; ok {
			validations = append(validations, codersdk.ValidationError{
				Field:  slugField,
				Detail: "must be unique within inline_mcp_servers",
			})
		} else {
			seenSlugs[server.Slug] = struct{}{}
		}

		if urlValidation := validateInlineMCPServerURL(i, server.URL, allowedIPRanges); urlValidation != nil {
			validations = append(validations, *urlValidation)
		}

		if len(server.Headers) > codersdk.MaxInlineMCPServerHeaders {
			validations = append(validations, codersdk.ValidationError{
				Field: fmt.Sprintf("inline_mcp_servers[%d].headers", i),
				Detail: fmt.Sprintf(
					"must contain at most %d headers",
					codersdk.MaxInlineMCPServerHeaders,
				),
			})
		}
		// Header names are case-insensitive identifiers: the transport
		// canonicalizes them on the wire, so two spellings of one name
		// would collide nondeterministically. Track the first spelling
		// of each canonical name and reject repeats.
		seen := make(map[string]string, len(server.Headers))
		for _, name := range slices.Sorted(maps.Keys(server.Headers)) {
			field := fmt.Sprintf("inline_mcp_servers[%d].headers[%s]", i, name)
			value := server.Headers[name]
			canonical := http.CanonicalHeaderKey(name)
			switch {
			case seen[canonical] != "":
				validations = append(validations, codersdk.ValidationError{
					Field:  field,
					Detail: fmt.Sprintf("header name duplicates %q; header names are case-insensitive", seen[canonical]),
				})
			case len(name) > codersdk.MaxInlineMCPServerHeaderNameBytes:
				validations = append(validations, codersdk.ValidationError{
					Field:  field,
					Detail: fmt.Sprintf("header name must not exceed %d bytes", codersdk.MaxInlineMCPServerHeaderNameBytes),
				})
			case !httpguts.ValidHeaderFieldName(name):
				validations = append(validations, codersdk.ValidationError{
					Field:  field,
					Detail: "header name is invalid",
				})
			case inlineMCPHeaderReserved(name):
				validations = append(validations, codersdk.ValidationError{
					Field:  field,
					Detail: "header name is reserved",
				})
			case len(value) < codersdk.MinInlineMCPServerHeaderValueBytes:
				validations = append(validations, codersdk.ValidationError{
					Field:  field,
					Detail: fmt.Sprintf("header value must be at least %d bytes", codersdk.MinInlineMCPServerHeaderValueBytes),
				})
			case len(value) > codersdk.MaxInlineMCPServerHeaderValueBytes:
				validations = append(validations, codersdk.ValidationError{
					Field:  field,
					Detail: fmt.Sprintf("header value must not exceed %d bytes", codersdk.MaxInlineMCPServerHeaderValueBytes),
				})
			case !httpguts.ValidHeaderFieldValue(value):
				validations = append(validations, codersdk.ValidationError{
					Field:  field,
					Detail: "header value is invalid",
				})
			}
			seen[canonical] = name
		}

		if len(server.ToolAllowList) > 0 && len(server.ToolDenyList) > 0 {
			validations = append(validations, codersdk.ValidationError{
				Field:  fmt.Sprintf("inline_mcp_servers[%d].tool_deny_list", i),
				Detail: "cannot be combined with tool_allow_list",
			})
		}
		validations = append(validations, validateInlineMCPToolFilter(i, "tool_allow_list", server.ToolAllowList)...)
		validations = append(validations, validateInlineMCPToolFilter(i, "tool_deny_list", server.ToolDenyList)...)
	}
	return validations
}

func validateInlineMCPServerURL(
	index int,
	rawURL string,
	allowedIPRanges []netip.Prefix,
) *codersdk.ValidationError {
	field := fmt.Sprintf("inline_mcp_servers[%d].url", index)
	invalid := func(detail string) *codersdk.ValidationError {
		return &codersdk.ValidationError{Field: field, Detail: detail}
	}
	if len(rawURL) == 0 {
		return invalid("is required")
	}
	if len(rawURL) > codersdk.MaxInlineMCPServerURLBytes {
		return invalid(fmt.Sprintf("must not exceed %d bytes", codersdk.MaxInlineMCPServerURLBytes))
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return invalid("must be a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return invalid("scheme must be http or https")
	}
	if parsed.Host == "" {
		return invalid("host is required")
	}
	if parsed.User != nil {
		return invalid("must not contain userinfo")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return invalid("must not contain a query string")
	}
	if parsed.Fragment != "" {
		return invalid("must not contain a fragment")
	}
	ip, err := netip.ParseAddr(parsed.Hostname())
	isIPLiteral := err == nil
	if isIPLiteral && safedial.CheckAddr(ip, safedial.WithAllowedPrefixes(allowedIPRanges...)) != nil {
		return invalid("host is in a private or reserved IP range")
	}
	// Plaintext is only allowed to an IP literal the operator allowlisted.
	// Hostnames resolve at connect time, so they cannot be trusted here.
	inAllowedRange := isIPLiteral && slices.ContainsFunc(allowedIPRanges, func(prefix netip.Prefix) bool {
		return prefix.Contains(ip.Unmap())
	})
	if parsed.Scheme == "http" && !inAllowedRange {
		return invalid("must use https, or http to an IP literal in an allowed private range")
	}
	return nil
}

func validateInlineMCPToolFilter(index int, name string, values []string) []codersdk.ValidationError {
	field := fmt.Sprintf("inline_mcp_servers[%d].%s", index, name)
	if len(values) > codersdk.MaxInlineMCPServerToolFilters {
		return []codersdk.ValidationError{{
			Field:  field,
			Detail: fmt.Sprintf("must contain at most %d tool names", codersdk.MaxInlineMCPServerToolFilters),
		}}
	}
	seen := make(map[string]struct{}, len(values))
	var validations []codersdk.ValidationError
	for i, value := range values {
		itemField := fmt.Sprintf("%s[%d]", field, i)
		_, duplicate := seen[value]
		switch {
		case value == "":
			validations = append(validations, codersdk.ValidationError{Field: itemField, Detail: "tool name must not be empty"})
		case len(value) > codersdk.MaxInlineMCPServerToolNameBytes:
			validations = append(validations, codersdk.ValidationError{
				Field:  itemField,
				Detail: fmt.Sprintf("tool name must not exceed %d bytes", codersdk.MaxInlineMCPServerToolNameBytes),
			})
		case strings.ContainsRune(value, '\x00'):
			validations = append(validations, codersdk.ValidationError{Field: itemField, Detail: "tool name must not contain null bytes"})
		case duplicate:
			validations = append(validations, codersdk.ValidationError{Field: itemField, Detail: "tool name must be unique within the list"})
		default:
			seen[value] = struct{}{}
		}
	}
	return validations
}

func inlineMCPServersSize(servers []codersdk.InlineMCPServerRequest) int {
	total := 0
	for _, server := range servers {
		total += len(server.Slug) + len(server.URL)
		for name, value := range server.Headers {
			total += len(name) + len(value)
		}
		for _, name := range server.ToolAllowList {
			total += len(name)
		}
		for _, name := range server.ToolDenyList {
			total += len(name)
		}
	}
	return total
}

func inlineMCPHeaderReserved(name string) bool {
	canonical := strings.ToLower(http.CanonicalHeaderKey(name))
	if strings.HasPrefix(canonical, "proxy-") || strings.HasPrefix(canonical, "x-coder-") {
		return true
	}
	switch canonical {
	case "host",
		"content-length",
		"connection",
		"transfer-encoding",
		"trailer",
		"upgrade",
		"te",
		"keep-alive",
		"accept",
		"accept-encoding",
		"content-type",
		"last-event-id",
		"mcp-protocol-version",
		"mcp-session-id":
		return true
	default:
		return false
	}
}
