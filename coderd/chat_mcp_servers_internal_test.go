package coderd

import (
	"fmt"
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestValidateChatMCPServers(t *testing.T) {
	t.Parallel()

	valid := codersdk.ChatMCPServerRequest{
		Slug: "private-tools",
		URL:  "https://mcp.example.com/v1",
		Headers: map[string]string{
			"Authorization": "Bearer private-token",
		},
		ToolAllowList: []string{"lookup"},
	}
	loopbackAllowed := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}

	tests := []struct {
		name       string
		servers    []codersdk.ChatMCPServerRequest
		allowed    []netip.Prefix
		wantField  string
		wantDetail string
	}{
		{name: "ValidHTTPS", servers: []codersdk.ChatMCPServerRequest{valid}},
		{
			name: "ValidAllowlistedHTTPWithHeaders",
			servers: []codersdk.ChatMCPServerRequest{{
				Slug:    "local",
				URL:     "http://127.0.0.1:3001/mcp",
				Headers: map[string]string{"Authorization": "Bearer test-only"},
			}},
			allowed: loopbackAllowed,
		},
		{
			name: "TooManyServers",
			servers: func() []codersdk.ChatMCPServerRequest {
				servers := make([]codersdk.ChatMCPServerRequest, codersdk.MaxChatMCPServers+1)
				for i := range servers {
					servers[i] = codersdk.ChatMCPServerRequest{
						Slug: fmt.Sprintf("server-%d", i),
						URL:  fmt.Sprintf("https://mcp-%d.example.com", i),
					}
				}
				return servers
			}(),
			wantField:  "mcp_servers",
			wantDetail: "at most",
		},
		{
			name:       "InvalidSlug",
			servers:    []codersdk.ChatMCPServerRequest{{Slug: "private tools", URL: valid.URL}},
			wantField:  "mcp_servers[0].slug",
			wantDetail: "letters",
		},
		{
			name: "DuplicateSlug",
			servers: []codersdk.ChatMCPServerRequest{
				valid,
				{Slug: valid.Slug, URL: "https://other.example.com/mcp"},
			},
			wantField:  "mcp_servers[1].slug",
			wantDetail: "unique",
		},
		{
			name:       "URLUserinfo",
			servers:    []codersdk.ChatMCPServerRequest{{Slug: "private", URL: "https://user:password@mcp.example.com"}},
			wantField:  "mcp_servers[0].url",
			wantDetail: "userinfo",
		},
		{
			name:       "URLQuery",
			servers:    []codersdk.ChatMCPServerRequest{{Slug: "private", URL: "https://mcp.example.com?token=secret"}},
			wantField:  "mcp_servers[0].url",
			wantDetail: "query",
		},
		{
			name:       "BlockedIPLiteral",
			servers:    []codersdk.ChatMCPServerRequest{{Slug: "private", URL: "http://169.254.169.254/latest/meta-data"}},
			wantField:  "mcp_servers[0].url",
			wantDetail: "private or reserved",
		},
		{
			name:       "HTTPHeaders",
			servers:    []codersdk.ChatMCPServerRequest{{Slug: "private", URL: "http://mcp.example.com", Headers: map[string]string{"Authorization": "Bearer secret"}}},
			wantField:  "mcp_servers[0].headers",
			wantDetail: "HTTPS",
		},
		{
			name:       "ReservedHeader",
			servers:    []codersdk.ChatMCPServerRequest{{Slug: "private", URL: valid.URL, Headers: map[string]string{"X-Coder-Chat-Id": "override"}}},
			wantField:  "mcp_servers[0].headers[X-Coder-Chat-Id]",
			wantDetail: "reserved",
		},
		{
			name:       "MCPProtocolHeader",
			servers:    []codersdk.ChatMCPServerRequest{{Slug: "private", URL: valid.URL, Headers: map[string]string{"Mcp-Session-Id": "fixed"}}},
			wantField:  "mcp_servers[0].headers[Mcp-Session-Id]",
			wantDetail: "reserved",
		},
		{
			name: "TooManyHeaders",
			servers: []codersdk.ChatMCPServerRequest{{
				Slug: "private",
				URL:  valid.URL,
				Headers: func() map[string]string {
					headers := make(map[string]string, maxChatMCPHeadersPerServer+1)
					for i := 0; i <= maxChatMCPHeadersPerServer; i++ {
						headers[fmt.Sprintf("X-Test-%d", i)] = "value"
					}
					return headers
				}(),
			}},
			wantField:  "mcp_servers[0].headers",
			wantDetail: "at most",
		},
		{
			name:       "HeaderValueTooLarge",
			servers:    []codersdk.ChatMCPServerRequest{{Slug: "private", URL: valid.URL, Headers: map[string]string{"Authorization": strings.Repeat("x", maxChatMCPHeaderValueBytes+1)}}},
			wantField:  "mcp_servers[0].headers[Authorization]",
			wantDetail: "must not exceed",
		},
		{
			name: "TooManyAllowedTools",
			servers: []codersdk.ChatMCPServerRequest{{
				Slug: "private",
				URL:  valid.URL,
				ToolAllowList: func() []string {
					names := make([]string, maxChatMCPToolFilters+1)
					for i := range names {
						names[i] = fmt.Sprintf("tool-%d", i)
					}
					return names
				}(),
			}},
			wantField:  "mcp_servers[0].tool_allow_list",
			wantDetail: "at most",
		},
		{
			name:       "URLTooLong",
			servers:    []codersdk.ChatMCPServerRequest{{Slug: "private", URL: "https://mcp.example.com/" + strings.Repeat("x", maxChatMCPServerURLBytes)}},
			wantField:  "mcp_servers[0].url",
			wantDetail: "must not exceed",
		},
		{
			name: "AllowAndDenyLists",
			servers: []codersdk.ChatMCPServerRequest{{
				Slug:          "private",
				URL:           valid.URL,
				ToolAllowList: []string{"lookup"},
				ToolDenyList:  []string{"delete"},
			}},
			wantField:  "mcp_servers[0].tool_deny_list",
			wantDetail: "cannot be combined",
		},
		{
			name: "AggregateSize",
			servers: []codersdk.ChatMCPServerRequest{{
				Slug: "private",
				URL:  valid.URL,
				Headers: map[string]string{
					"Authorization": strings.Repeat("x", codersdk.MaxChatMCPServersBytes),
				},
			}},
			wantField:  "mcp_servers",
			wantDetail: "total size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			validations := validateChatMCPServers(tt.servers, tt.allowed)
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
