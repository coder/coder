package coderd

import (
	"fmt"
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/codersdk"
)

func TestValidateInlineMCPServers(t *testing.T) {
	t.Parallel()

	valid := codersdk.InlineMCPServerRequest{
		Slug: "private-tools",
		URL:  "https://mcp.example.com/v1",
		Headers: map[string]string{
			"Authorization": "Bearer private-token",
		},
		ToolAllowList: []string{"lookup"},
	}
	loopbackAllowed := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	longHeaderName := strings.Repeat("X", codersdk.MaxInlineMCPServerHeaderNameBytes+1)

	tests := []struct {
		name       string
		servers    []codersdk.InlineMCPServerRequest
		allowed    []netip.Prefix
		wantField  string
		wantDetail string
	}{
		{name: "ValidHTTPS", servers: []codersdk.InlineMCPServerRequest{valid}},
		{
			name: "ValidAllowlistedHTTPWithHeaders",
			servers: []codersdk.InlineMCPServerRequest{{
				Slug:    "local",
				URL:     "http://127.0.0.1:3001/mcp",
				Headers: map[string]string{"Authorization": "Bearer test-only"},
			}},
			allowed: loopbackAllowed,
		},
		{
			name: "TooManyServers",
			servers: func() []codersdk.InlineMCPServerRequest {
				servers := make([]codersdk.InlineMCPServerRequest, codersdk.MaxInlineMCPServers+1)
				for i := range servers {
					servers[i] = codersdk.InlineMCPServerRequest{
						Slug: fmt.Sprintf("server-%d", i),
						URL:  fmt.Sprintf("https://mcp-%d.example.com", i),
					}
				}
				return servers
			}(),
			wantField:  "inline_mcp_servers",
			wantDetail: "at most",
		},
		{
			name:       "InvalidSlug",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private tools", URL: valid.URL}},
			wantField:  "inline_mcp_servers[0].slug",
			wantDetail: "letters",
		},
		{
			name: "DuplicateSlug",
			servers: []codersdk.InlineMCPServerRequest{
				valid,
				{Slug: valid.Slug, URL: "https://other.example.com/mcp"},
			},
			wantField:  "inline_mcp_servers[1].slug",
			wantDetail: "unique",
		},
		{
			name:       "URLEmpty",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private", URL: ""}},
			wantField:  "inline_mcp_servers[0].url",
			wantDetail: "is required",
		},
		{
			name:       "URLUnparseable",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private", URL: "https://mcp.example.com/%zz"}},
			wantField:  "inline_mcp_servers[0].url",
			wantDetail: "valid URL",
		},
		{
			name:       "URLScheme",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private", URL: "ftp://mcp.example.com/mcp"}},
			wantField:  "inline_mcp_servers[0].url",
			wantDetail: "scheme",
		},
		{
			name:       "URLMissingHost",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private", URL: "https:///mcp"}},
			wantField:  "inline_mcp_servers[0].url",
			wantDetail: "host is required",
		},
		{
			name:       "URLFragment",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private", URL: "https://mcp.example.com/mcp#token"}},
			wantField:  "inline_mcp_servers[0].url",
			wantDetail: "fragment",
		},
		{
			name:       "URLUserinfo",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private", URL: "https://user:password@mcp.example.com"}},
			wantField:  "inline_mcp_servers[0].url",
			wantDetail: "userinfo",
		},
		{
			name:       "URLQuery",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private", URL: "https://mcp.example.com?token=secret"}},
			wantField:  "inline_mcp_servers[0].url",
			wantDetail: "query",
		},
		{
			// Only coderd code can attach an internal MCP server.
			name:       "InternalScheme",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private", URL: mcpclient.InternalScheme + "://slack"}},
			wantField:  "inline_mcp_servers[0].url",
			wantDetail: "scheme must be http or https",
		},
		{
			name:       "BlockedIPLiteral",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private", URL: "http://169.254.169.254/latest/meta-data"}},
			wantField:  "inline_mcp_servers[0].url",
			wantDetail: "private or reserved",
		},
		{
			name:       "PrivateIPOutsideAllowlist",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private", URL: "https://10.0.0.1/mcp"}},
			allowed:    loopbackAllowed,
			wantField:  "inline_mcp_servers[0].url",
			wantDetail: "private or reserved",
		},
		{
			name:       "HTTPHostname",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private", URL: "http://mcp.example.com"}},
			allowed:    loopbackAllowed,
			wantField:  "inline_mcp_servers[0].url",
			wantDetail: "must use https",
		},
		{
			name:       "HTTPPublicIPLiteral",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private", URL: "http://8.8.8.8/mcp"}},
			allowed:    loopbackAllowed,
			wantField:  "inline_mcp_servers[0].url",
			wantDetail: "must use https",
		},
		{
			name:       "ReservedHeader",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private", URL: valid.URL, Headers: map[string]string{"X-Coder-Chat-Id": "override"}}},
			wantField:  "inline_mcp_servers[0].headers[X-Coder-Chat-Id]",
			wantDetail: "reserved",
		},
		{
			name:       "MCPProtocolHeader",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private", URL: valid.URL, Headers: map[string]string{"Mcp-Session-Id": "fixed-session"}}},
			wantField:  "inline_mcp_servers[0].headers[Mcp-Session-Id]",
			wantDetail: "reserved",
		},
		{
			name:       "HeaderNameTooLong",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private", URL: valid.URL, Headers: map[string]string{longHeaderName: "header-value"}}},
			wantField:  "inline_mcp_servers[0].headers[" + longHeaderName + "]",
			wantDetail: "must not exceed",
		},
		{
			name:       "HeaderNameInvalid",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private", URL: valid.URL, Headers: map[string]string{"Bad Header": "header-value"}}},
			wantField:  "inline_mcp_servers[0].headers[Bad Header]",
			wantDetail: "header name is invalid",
		},
		{
			name:       "HeaderValueInvalid",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private", URL: valid.URL, Headers: map[string]string{"Authorization": "Bearer x\r\nX-Injected: 1"}}},
			wantField:  "inline_mcp_servers[0].headers[Authorization]",
			wantDetail: "header value is invalid",
		},
		{
			name: "TooManyHeaders",
			servers: []codersdk.InlineMCPServerRequest{{
				Slug: "private",
				URL:  valid.URL,
				Headers: func() map[string]string {
					headers := make(map[string]string, codersdk.MaxInlineMCPServerHeaders+1)
					for i := 0; i <= codersdk.MaxInlineMCPServerHeaders; i++ {
						headers[fmt.Sprintf("X-Test-%d", i)] = "header-value"
					}
					return headers
				}(),
			}},
			wantField:  "inline_mcp_servers[0].headers",
			wantDetail: "at most",
		},
		{
			name: "DuplicateHeaderNameCase",
			servers: []codersdk.InlineMCPServerRequest{{
				Slug: "private",
				URL:  valid.URL,
				Headers: map[string]string{
					"Authorization": "Bearer aaaaaaaa",
					"authorization": "Bearer bbbbbbbb",
				},
			}},
			wantField:  "inline_mcp_servers[0].headers[authorization]",
			wantDetail: `duplicates "Authorization"`,
		},
		{
			name:       "HeaderValueTooLarge",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private", URL: valid.URL, Headers: map[string]string{"Authorization": strings.Repeat("x", codersdk.MaxInlineMCPServerHeaderValueBytes+1)}}},
			wantField:  "inline_mcp_servers[0].headers[Authorization]",
			wantDetail: "must not exceed",
		},
		{
			name: "TooManyAllowedTools",
			servers: []codersdk.InlineMCPServerRequest{{
				Slug: "private",
				URL:  valid.URL,
				ToolAllowList: func() []string {
					names := make([]string, codersdk.MaxInlineMCPServerToolFilters+1)
					for i := range names {
						names[i] = fmt.Sprintf("tool-%d", i)
					}
					return names
				}(),
			}},
			wantField:  "inline_mcp_servers[0].tool_allow_list",
			wantDetail: "at most",
		},
		{
			name:       "EmptyToolName",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private", URL: valid.URL, ToolAllowList: []string{""}}},
			wantField:  "inline_mcp_servers[0].tool_allow_list[0]",
			wantDetail: "must not be empty",
		},
		{
			name:       "ToolNameTooLong",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private", URL: valid.URL, ToolDenyList: []string{strings.Repeat("x", codersdk.MaxInlineMCPServerToolNameBytes+1)}}},
			wantField:  "inline_mcp_servers[0].tool_deny_list[0]",
			wantDetail: "must not exceed",
		},
		{
			name:       "ToolNameNullByte",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private", URL: valid.URL, ToolAllowList: []string{"look\x00up"}}},
			wantField:  "inline_mcp_servers[0].tool_allow_list[0]",
			wantDetail: "null bytes",
		},
		{
			name:       "DuplicateToolName",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private", URL: valid.URL, ToolDenyList: []string{"delete", "delete"}}},
			wantField:  "inline_mcp_servers[0].tool_deny_list[1]",
			wantDetail: "unique",
		},
		{
			name:       "URLTooLong",
			servers:    []codersdk.InlineMCPServerRequest{{Slug: "private", URL: "https://mcp.example.com/" + strings.Repeat("x", codersdk.MaxInlineMCPServerURLBytes)}},
			wantField:  "inline_mcp_servers[0].url",
			wantDetail: "must not exceed",
		},
		{
			name: "AllowAndDenyLists",
			servers: []codersdk.InlineMCPServerRequest{{
				Slug:          "private",
				URL:           valid.URL,
				ToolAllowList: []string{"lookup"},
				ToolDenyList:  []string{"delete"},
			}},
			wantField:  "inline_mcp_servers[0].tool_deny_list",
			wantDetail: "cannot be combined",
		},
		{
			name: "AggregateSize",
			servers: []codersdk.InlineMCPServerRequest{{
				Slug: "private",
				URL:  valid.URL,
				Headers: map[string]string{
					"Authorization": strings.Repeat("x", codersdk.MaxInlineMCPServersBytes),
				},
			}},
			wantField:  "inline_mcp_servers",
			wantDetail: "total size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			validations := validateInlineMCPServers(tt.servers, tt.allowed)
			if tt.wantField == "" {
				require.Empty(t, validations)
				return
			}
			require.NotEmpty(t, validations)
			require.Equal(t, tt.wantField, validations[0].Field)
			require.Contains(t, validations[0].Detail, tt.wantDetail)
		})
	}
}
