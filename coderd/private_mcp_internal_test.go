package coderd

import (
	"fmt"
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestValidatePrivateMCPServerConfigs(t *testing.T) {
	t.Parallel()

	valid := codersdk.PrivateMCPServerConfig{
		Name: "private-tools",
		URL:  "https://mcp.example.com/v1",
		Headers: map[string]string{
			"Authorization": "Bearer private-token",
		},
		ToolAllowList: []string{"lookup"},
	}
	loopbackAllowed := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}

	tests := []struct {
		name       string
		configs    []codersdk.PrivateMCPServerConfig
		allowed    []netip.Prefix
		wantField  string
		wantDetail string
	}{
		{name: "ValidHTTPS", configs: []codersdk.PrivateMCPServerConfig{valid}},
		{
			name: "ValidAllowlistedHTTPWithHeaders",
			configs: []codersdk.PrivateMCPServerConfig{{
				Name:    "local",
				URL:     "http://127.0.0.1:3001/mcp",
				Headers: map[string]string{"Authorization": "Bearer test-only"},
			}},
			allowed: loopbackAllowed,
		},
		{
			name: "TooManyServers",
			configs: func() []codersdk.PrivateMCPServerConfig {
				configs := make([]codersdk.PrivateMCPServerConfig, codersdk.MaxPrivateMCPServerConfigs+1)
				for i := range configs {
					configs[i] = codersdk.PrivateMCPServerConfig{
						Name: fmt.Sprintf("server-%d", i),
						URL:  fmt.Sprintf("https://mcp-%d.example.com", i),
					}
				}
				return configs
			}(),
			wantField:  "private_mcp_server_configs",
			wantDetail: "at most",
		},
		{
			name:       "InvalidName",
			configs:    []codersdk.PrivateMCPServerConfig{{Name: "private tools", URL: valid.URL}},
			wantField:  "private_mcp_server_configs[0].name",
			wantDetail: "letters",
		},
		{
			name: "DuplicateName",
			configs: []codersdk.PrivateMCPServerConfig{
				valid,
				{Name: valid.Name, URL: "https://other.example.com/mcp"},
			},
			wantField:  "private_mcp_server_configs[1].name",
			wantDetail: "unique",
		},
		{
			name:       "URLUserinfo",
			configs:    []codersdk.PrivateMCPServerConfig{{Name: "private", URL: "https://user:password@mcp.example.com"}},
			wantField:  "private_mcp_server_configs[0].url",
			wantDetail: "userinfo",
		},
		{
			name:       "URLQuery",
			configs:    []codersdk.PrivateMCPServerConfig{{Name: "private", URL: "https://mcp.example.com?token=secret"}},
			wantField:  "private_mcp_server_configs[0].url",
			wantDetail: "query",
		},
		{
			name:       "BlockedIPLiteral",
			configs:    []codersdk.PrivateMCPServerConfig{{Name: "private", URL: "http://169.254.169.254/latest/meta-data"}},
			wantField:  "private_mcp_server_configs[0].url",
			wantDetail: "private or reserved",
		},
		{
			name:       "HTTPHeaders",
			configs:    []codersdk.PrivateMCPServerConfig{{Name: "private", URL: "http://mcp.example.com", Headers: map[string]string{"Authorization": "Bearer secret"}}},
			wantField:  "private_mcp_server_configs[0].headers",
			wantDetail: "HTTPS",
		},
		{
			name:       "ReservedHeader",
			configs:    []codersdk.PrivateMCPServerConfig{{Name: "private", URL: valid.URL, Headers: map[string]string{"X-Coder-Chat-Id": "override"}}},
			wantField:  "private_mcp_server_configs[0].headers[X-Coder-Chat-Id]",
			wantDetail: "reserved",
		},
		{
			name:       "MCPProtocolHeader",
			configs:    []codersdk.PrivateMCPServerConfig{{Name: "private", URL: valid.URL, Headers: map[string]string{"Mcp-Session-Id": "fixed"}}},
			wantField:  "private_mcp_server_configs[0].headers[Mcp-Session-Id]",
			wantDetail: "reserved",
		},
		{
			name: "TooManyHeaders",
			configs: []codersdk.PrivateMCPServerConfig{{
				Name: "private",
				URL:  valid.URL,
				Headers: func() map[string]string {
					headers := make(map[string]string, maxPrivateMCPHeadersPerServer+1)
					for i := 0; i <= maxPrivateMCPHeadersPerServer; i++ {
						headers[fmt.Sprintf("X-Test-%d", i)] = "value"
					}
					return headers
				}(),
			}},
			wantField:  "private_mcp_server_configs[0].headers",
			wantDetail: "at most",
		},
		{
			name:       "HeaderValueTooLarge",
			configs:    []codersdk.PrivateMCPServerConfig{{Name: "private", URL: valid.URL, Headers: map[string]string{"Authorization": strings.Repeat("x", maxPrivateMCPHeaderValueBytes+1)}}},
			wantField:  "private_mcp_server_configs[0].headers[Authorization]",
			wantDetail: "must not exceed",
		},
		{
			name: "TooManyAllowedTools",
			configs: []codersdk.PrivateMCPServerConfig{{
				Name: "private",
				URL:  valid.URL,
				ToolAllowList: func() []string {
					names := make([]string, maxPrivateMCPToolFilters+1)
					for i := range names {
						names[i] = fmt.Sprintf("tool-%d", i)
					}
					return names
				}(),
			}},
			wantField:  "private_mcp_server_configs[0].tool_allow_list",
			wantDetail: "at most",
		},
		{
			name:       "URLTooLong",
			configs:    []codersdk.PrivateMCPServerConfig{{Name: "private", URL: "https://mcp.example.com/" + strings.Repeat("x", maxPrivateMCPServerURLBytes)}},
			wantField:  "private_mcp_server_configs[0].url",
			wantDetail: "must not exceed",
		},
		{
			name: "AllowAndDenyLists",
			configs: []codersdk.PrivateMCPServerConfig{{
				Name:          "private",
				URL:           valid.URL,
				ToolAllowList: []string{"lookup"},
				ToolDenyList:  []string{"delete"},
			}},
			wantField:  "private_mcp_server_configs[0].tool_deny_list",
			wantDetail: "cannot be combined",
		},
		{
			name: "AggregateSize",
			configs: []codersdk.PrivateMCPServerConfig{{
				Name: "private",
				URL:  valid.URL,
				Headers: map[string]string{
					"Authorization": strings.Repeat("x", codersdk.MaxPrivateMCPServerConfigBytes),
				},
			}},
			wantField:  "private_mcp_server_configs",
			wantDetail: "total size",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			validations := validatePrivateMCPServerConfigs(tt.configs, tt.allowed)
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
