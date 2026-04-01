package vpn

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
	"tailscale.com/net/dns"
	"tailscale.com/util/dnsname"
)

func TestConvertDNSConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    dns.OSConfig
		expected *NetworkSettingsRequest_DNSSettings
	}{
		{
			name: "Basic",
			input: dns.OSConfig{
				Nameservers: []netip.Addr{
					netip.MustParseAddr("1.1.1.1"),
					netip.MustParseAddr("8.8.8.8"),
				},
				SearchDomains: []dnsname.FQDN{
					"example.com.",
					"test.local.",
				},
				MatchDomains: []dnsname.FQDN{
					"internal.domain.",
				},
			},
			expected: &NetworkSettingsRequest_DNSSettings{
				Servers:              []string{"1.1.1.1", "8.8.8.8"},
				SearchDomains:        []string{"example.com", "test.local"},
				DomainName:           "coder",
				MatchDomains:         []string{"internal.domain"},
				MatchDomainsNoSearch: false,
			},
		},
		{
			name: "Empty",
			input: dns.OSConfig{
				Nameservers:   []netip.Addr{},
				SearchDomains: []dnsname.FQDN{},
				MatchDomains:  []dnsname.FQDN{},
			},
			expected: &NetworkSettingsRequest_DNSSettings{
				Servers:              []string{},
				SearchDomains:        []string{},
				DomainName:           "coder",
				MatchDomains:         []string{},
				MatchDomainsNoSearch: false,
			},
		},
	}

	//nolint:paralleltest // outdated rule
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := convertDNSConfig(tt.input)
			require.Equal(t, tt.expected.Servers, result.Servers)
			require.Equal(t, tt.expected.SearchDomains, result.SearchDomains)
			require.Equal(t, tt.expected.DomainName, result.DomainName)
			require.Equal(t, tt.expected.MatchDomains, result.MatchDomains)
			require.Equal(t, tt.expected.MatchDomainsNoSearch, result.MatchDomainsNoSearch)
		})
	}
}
