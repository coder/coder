package coderd

import (
	"fmt"
	"maps"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/net/http/httpguts"

	"github.com/coder/coder/v2/codersdk"
)

const (
	maxPrivateMCPServerNameBytes  = 32
	maxPrivateMCPServerURLBytes   = 2048
	maxPrivateMCPHeadersPerServer = 16
	maxPrivateMCPHeaderNameBytes  = 128
	maxPrivateMCPHeaderValueBytes = 8 * 1024
	maxPrivateMCPToolFilters      = 64
	maxPrivateMCPToolNameBytes    = 128
)

var privateMCPServerNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,31}$`)

func validatePrivateMCPServerConfigs(
	configs []codersdk.PrivateMCPServerConfig,
	allowedIPRanges []netip.Prefix,
) []codersdk.ValidationError {
	if len(configs) > codersdk.MaxPrivateMCPServerConfigs {
		return []codersdk.ValidationError{{
			Field:  "private_mcp_server_configs",
			Detail: fmt.Sprintf("must contain at most %d servers", codersdk.MaxPrivateMCPServerConfigs),
		}}
	}

	if privateMCPServerConfigSize(configs) > codersdk.MaxPrivateMCPServerConfigBytes {
		return []codersdk.ValidationError{{
			Field: "private_mcp_server_configs",
			Detail: fmt.Sprintf(
				"total size must not exceed %d bytes",
				codersdk.MaxPrivateMCPServerConfigBytes,
			),
		}}
	}

	seenNames := make(map[string]struct{}, len(configs))
	var validations []codersdk.ValidationError
	for i, config := range configs {
		nameField := fmt.Sprintf("private_mcp_server_configs[%d].name", i)
		if !privateMCPServerNamePattern.MatchString(config.Name) {
			validations = append(validations, codersdk.ValidationError{
				Field: nameField,
				Detail: fmt.Sprintf(
					"must be 1 to %d ASCII letters, numbers, underscores, or hyphens, and must start with a letter or number",
					maxPrivateMCPServerNameBytes,
				),
			})
		} else if _, ok := seenNames[config.Name]; ok {
			validations = append(validations, codersdk.ValidationError{
				Field:  nameField,
				Detail: "must be unique within private_mcp_server_configs",
			})
		} else {
			seenNames[config.Name] = struct{}{}
		}

		parsedURL, urlValidation := validatePrivateMCPServerURL(i, config.URL, allowedIPRanges)
		if urlValidation != nil {
			validations = append(validations, *urlValidation)
		}

		if len(config.Headers) > maxPrivateMCPHeadersPerServer {
			validations = append(validations, codersdk.ValidationError{
				Field: fmt.Sprintf("private_mcp_server_configs[%d].headers", i),
				Detail: fmt.Sprintf(
					"must contain at most %d headers",
					maxPrivateMCPHeadersPerServer,
				),
			})
		}
		for _, name := range slices.Sorted(maps.Keys(config.Headers)) {
			field := fmt.Sprintf("private_mcp_server_configs[%d].headers[%s]", i, name)
			value := config.Headers[name]
			switch {
			case len(name) > maxPrivateMCPHeaderNameBytes:
				validations = append(validations, codersdk.ValidationError{
					Field:  field,
					Detail: fmt.Sprintf("header name must not exceed %d bytes", maxPrivateMCPHeaderNameBytes),
				})
			case !httpguts.ValidHeaderFieldName(name):
				validations = append(validations, codersdk.ValidationError{
					Field:  field,
					Detail: "header name is invalid",
				})
			case privateMCPHeaderReserved(name):
				validations = append(validations, codersdk.ValidationError{
					Field:  field,
					Detail: "header name is reserved",
				})
			case len(value) > maxPrivateMCPHeaderValueBytes:
				validations = append(validations, codersdk.ValidationError{
					Field:  field,
					Detail: fmt.Sprintf("header value must not exceed %d bytes", maxPrivateMCPHeaderValueBytes),
				})
			case !httpguts.ValidHeaderFieldValue(value):
				validations = append(validations, codersdk.ValidationError{
					Field:  field,
					Detail: "header value is invalid",
				})
			}
		}

		if parsedURL != nil && parsedURL.Scheme == "http" && len(config.Headers) > 0 && !privateMCPURLUsesAllowedIPLiteral(parsedURL, allowedIPRanges) {
			validations = append(validations, codersdk.ValidationError{
				Field:  fmt.Sprintf("private_mcp_server_configs[%d].headers", i),
				Detail: "headers require an HTTPS server URL",
			})
		}

		if len(config.ToolAllowList) > 0 && len(config.ToolDenyList) > 0 {
			validations = append(validations, codersdk.ValidationError{
				Field:  fmt.Sprintf("private_mcp_server_configs[%d].tool_deny_list", i),
				Detail: "cannot be combined with tool_allow_list",
			})
		}
		validations = append(validations, validatePrivateMCPToolFilter(i, "tool_allow_list", config.ToolAllowList)...)
		validations = append(validations, validatePrivateMCPToolFilter(i, "tool_deny_list", config.ToolDenyList)...)
	}
	return validations
}

func validatePrivateMCPServerURL(
	index int,
	rawURL string,
	allowedIPRanges []netip.Prefix,
) (*url.URL, *codersdk.ValidationError) {
	field := fmt.Sprintf("private_mcp_server_configs[%d].url", index)
	invalid := func(detail string) (*url.URL, *codersdk.ValidationError) {
		return nil, &codersdk.ValidationError{Field: field, Detail: detail}
	}
	if len(rawURL) == 0 {
		return invalid("is required")
	}
	if len(rawURL) > maxPrivateMCPServerURLBytes {
		return invalid(fmt.Sprintf("must not exceed %d bytes", maxPrivateMCPServerURLBytes))
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
	if ip, err := netip.ParseAddr(parsed.Hostname()); err == nil && isBlockedMCPDiscoveryAddr(ip, allowedIPRanges) {
		return invalid("host is in a private or reserved IP range")
	}
	return parsed, nil
}

func validatePrivateMCPToolFilter(index int, name string, values []string) []codersdk.ValidationError {
	field := fmt.Sprintf("private_mcp_server_configs[%d].%s", index, name)
	if len(values) > maxPrivateMCPToolFilters {
		return []codersdk.ValidationError{{
			Field:  field,
			Detail: fmt.Sprintf("must contain at most %d tool names", maxPrivateMCPToolFilters),
		}}
	}
	seen := make(map[string]struct{}, len(values))
	var validations []codersdk.ValidationError
	for i, value := range values {
		itemField := fmt.Sprintf("%s[%d]", field, i)
		switch {
		case value == "":
			validations = append(validations, codersdk.ValidationError{Field: itemField, Detail: "tool name must not be empty"})
		case len(value) > maxPrivateMCPToolNameBytes:
			validations = append(validations, codersdk.ValidationError{
				Field:  itemField,
				Detail: fmt.Sprintf("tool name must not exceed %d bytes", maxPrivateMCPToolNameBytes),
			})
		case strings.ContainsRune(value, '\x00'):
			validations = append(validations, codersdk.ValidationError{Field: itemField, Detail: "tool name must not contain null bytes"})
		case func() bool {
			_, ok := seen[value]
			return ok
		}():
			validations = append(validations, codersdk.ValidationError{Field: itemField, Detail: "tool name must be unique within the list"})
		default:
			seen[value] = struct{}{}
		}
	}
	return validations
}

func privateMCPServerConfigSize(configs []codersdk.PrivateMCPServerConfig) int {
	total := 0
	for _, config := range configs {
		total += len(config.Name) + len(config.URL)
		for name, value := range config.Headers {
			total += len(name) + len(value)
		}
		for _, name := range config.ToolAllowList {
			total += len(name)
		}
		for _, name := range config.ToolDenyList {
			total += len(name)
		}
	}
	return total
}

func privateMCPURLUsesAllowedIPLiteral(parsed *url.URL, allowedIPRanges []netip.Prefix) bool {
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

func privateMCPHeaderReserved(name string) bool {
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
