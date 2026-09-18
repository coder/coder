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

const (
	maxChatMCPServerSlugBytes  = 32
	maxChatMCPServerURLBytes   = 2048
	maxChatMCPHeadersPerServer = 16
	maxChatMCPHeaderNameBytes  = 128
	maxChatMCPHeaderValueBytes = 8 * 1024
	maxChatMCPToolFilters      = 64
	maxChatMCPToolNameBytes    = 128
)

var chatMCPServerSlugPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,31}$`)

func writeChatCallerSuppliedToolsDisabled(ctx context.Context, rw http.ResponseWriter) {
	httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
		Message: "Caller-supplied tools are disabled on this deployment.",
		Detail:  "The server runs with --disable-chat-caller-supplied-tools. Remove unsafe_dynamic_tools and mcp_servers from the request.",
	})
}

func writeChatMCPServersExperimentRequired(ctx context.Context, rw http.ResponseWriter) {
	httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
		Message: "Chat-attached MCP servers are not enabled on this deployment.",
		Detail:  "Enable the chat-mcp-servers experiment.",
	})
}

func writeChatMCPServersInvalid(ctx context.Context, rw http.ResponseWriter, validations []codersdk.ValidationError) {
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message:     "Invalid mcp_servers.",
		Validations: validations,
	})
}

func validateChatMCPServers(
	servers []codersdk.ChatMCPServerRequest,
	allowedIPRanges []netip.Prefix,
) []codersdk.ValidationError {
	if len(servers) > codersdk.MaxChatMCPServers {
		return []codersdk.ValidationError{{
			Field:  "mcp_servers",
			Detail: fmt.Sprintf("must contain at most %d servers", codersdk.MaxChatMCPServers),
		}}
	}

	if chatMCPServersSize(servers) > codersdk.MaxChatMCPServersBytes {
		return []codersdk.ValidationError{{
			Field: "mcp_servers",
			Detail: fmt.Sprintf(
				"total size must not exceed %d bytes",
				codersdk.MaxChatMCPServersBytes,
			),
		}}
	}

	seenSlugs := make(map[string]struct{}, len(servers))
	var validations []codersdk.ValidationError
	for i, server := range servers {
		slugField := fmt.Sprintf("mcp_servers[%d].slug", i)
		if !chatMCPServerSlugPattern.MatchString(server.Slug) {
			validations = append(validations, codersdk.ValidationError{
				Field: slugField,
				Detail: fmt.Sprintf(
					"must be 1 to %d ASCII letters, numbers, underscores, or hyphens, and must start with a letter or number",
					maxChatMCPServerSlugBytes,
				),
			})
		} else if _, ok := seenSlugs[server.Slug]; ok {
			validations = append(validations, codersdk.ValidationError{
				Field:  slugField,
				Detail: "must be unique within mcp_servers",
			})
		} else {
			seenSlugs[server.Slug] = struct{}{}
		}

		parsedURL, urlValidation := validateChatMCPServerURL(i, server.URL, allowedIPRanges)
		if urlValidation != nil {
			validations = append(validations, *urlValidation)
		}

		if len(server.Headers) > maxChatMCPHeadersPerServer {
			validations = append(validations, codersdk.ValidationError{
				Field: fmt.Sprintf("mcp_servers[%d].headers", i),
				Detail: fmt.Sprintf(
					"must contain at most %d headers",
					maxChatMCPHeadersPerServer,
				),
			})
		}
		for _, name := range slices.Sorted(maps.Keys(server.Headers)) {
			field := fmt.Sprintf("mcp_servers[%d].headers[%s]", i, name)
			value := server.Headers[name]
			switch {
			case len(name) > maxChatMCPHeaderNameBytes:
				validations = append(validations, codersdk.ValidationError{
					Field:  field,
					Detail: fmt.Sprintf("header name must not exceed %d bytes", maxChatMCPHeaderNameBytes),
				})
			case !httpguts.ValidHeaderFieldName(name):
				validations = append(validations, codersdk.ValidationError{
					Field:  field,
					Detail: "header name is invalid",
				})
			case chatMCPHeaderReserved(name):
				validations = append(validations, codersdk.ValidationError{
					Field:  field,
					Detail: "header name is reserved",
				})
			case len(value) > maxChatMCPHeaderValueBytes:
				validations = append(validations, codersdk.ValidationError{
					Field:  field,
					Detail: fmt.Sprintf("header value must not exceed %d bytes", maxChatMCPHeaderValueBytes),
				})
			case !httpguts.ValidHeaderFieldValue(value):
				validations = append(validations, codersdk.ValidationError{
					Field:  field,
					Detail: "header value is invalid",
				})
			}
		}

		if parsedURL != nil && parsedURL.Scheme == "http" && len(server.Headers) > 0 && !chatMCPURLUsesAllowedIPLiteral(parsedURL, allowedIPRanges) {
			validations = append(validations, codersdk.ValidationError{
				Field:  fmt.Sprintf("mcp_servers[%d].headers", i),
				Detail: "headers require an HTTPS server URL",
			})
		}

		if len(server.ToolAllowList) > 0 && len(server.ToolDenyList) > 0 {
			validations = append(validations, codersdk.ValidationError{
				Field:  fmt.Sprintf("mcp_servers[%d].tool_deny_list", i),
				Detail: "cannot be combined with tool_allow_list",
			})
		}
		validations = append(validations, validateChatMCPToolFilter(i, "tool_allow_list", server.ToolAllowList)...)
		validations = append(validations, validateChatMCPToolFilter(i, "tool_deny_list", server.ToolDenyList)...)
	}
	return validations
}

func validateChatMCPServerURL(
	index int,
	rawURL string,
	allowedIPRanges []netip.Prefix,
) (*url.URL, *codersdk.ValidationError) {
	field := fmt.Sprintf("mcp_servers[%d].url", index)
	invalid := func(detail string) (*url.URL, *codersdk.ValidationError) {
		return nil, &codersdk.ValidationError{Field: field, Detail: detail}
	}
	if len(rawURL) == 0 {
		return invalid("is required")
	}
	if len(rawURL) > maxChatMCPServerURLBytes {
		return invalid(fmt.Sprintf("must not exceed %d bytes", maxChatMCPServerURLBytes))
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
	if ip, err := netip.ParseAddr(parsed.Hostname()); err == nil && safedial.CheckAddr(ip, safedial.WithAllowedPrefixes(allowedIPRanges...)) != nil {
		return invalid("host is in a private or reserved IP range")
	}
	return parsed, nil
}

func validateChatMCPToolFilter(index int, name string, values []string) []codersdk.ValidationError {
	field := fmt.Sprintf("mcp_servers[%d].%s", index, name)
	if len(values) > maxChatMCPToolFilters {
		return []codersdk.ValidationError{{
			Field:  field,
			Detail: fmt.Sprintf("must contain at most %d tool names", maxChatMCPToolFilters),
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
		case len(value) > maxChatMCPToolNameBytes:
			validations = append(validations, codersdk.ValidationError{
				Field:  itemField,
				Detail: fmt.Sprintf("tool name must not exceed %d bytes", maxChatMCPToolNameBytes),
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

func chatMCPServersSize(servers []codersdk.ChatMCPServerRequest) int {
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

func chatMCPURLUsesAllowedIPLiteral(parsed *url.URL, allowedIPRanges []netip.Prefix) bool {
	ip, err := netip.ParseAddr(parsed.Hostname())
	if err != nil {
		return false
	}
	ip = ip.Unmap()
	for _, prefix := range allowedIPRanges {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

func chatMCPHeaderReserved(name string) bool {
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
