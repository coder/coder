package vpn

import "tailscale.com/net/dns"

func NewVPNManager(t *Tunnel) dns.OSConfigurator {
	return &vpnDNSManager{tunnel: t}
}

type vpnDNSManager struct {
	tunnel *Tunnel
}

func (v *vpnDNSManager) SetDNS(cfg dns.OSConfig) error {
	servers := make([]string, 0, len(cfg.Nameservers))
	for _, ns := range cfg.Nameservers {
		servers = append(servers, ns.String())
	}
	searchDomains := make([]string, 0, len(cfg.SearchDomains))
	for _, domain := range cfg.SearchDomains {
		searchDomains = append(searchDomains, domain.WithoutTrailingDot())
	}
	matchDomains := make([]string, 0, len(cfg.MatchDomains))
	for _, domain := range cfg.MatchDomains {
		matchDomains = append(matchDomains, domain.WithoutTrailingDot())
	}
	return v.tunnel.ApplyNetworkSettings(v.tunnel.ctx, &NetworkSettingsRequest{
		DnsSettings: &NetworkSettingsRequest_DNSSettings{
			Servers:              servers,
			SearchDomains:        searchDomains,
			DomainName:           "coder",
			MatchDomains:         matchDomains,
			MatchDomainsNoSearch: false,
		},
	})
}

func (vpnDNSManager) GetBaseConfig() (dns.OSConfig, error) {
	// Tailscale calls this function to blend the OS's DNS configuration with
	// it's own, so this is only called if `SupportsSplitDNS` returns false.
	return dns.OSConfig{}, dns.ErrGetBaseConfigNotSupported
}

func (*vpnDNSManager) SupportsSplitDNS() bool {
	// macOS & Windows 10+ support split DNS, so we'll assume all CoderVPN
	// clients do too.
	return true
}

// Close implements dns.OSConfigurator.
func (*vpnDNSManager) Close() error {
	// There's no cleanup that we need to initiate from within the dylib.
	return nil
}

var _ dns.OSConfigurator = (*vpnDNSManager)(nil)
