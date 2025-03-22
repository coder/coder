package vpn

import "tailscale.com/net/dns"

func NewDNSConfigurator(t *Tunnel) dns.OSConfigurator {
	// Default to "coder" if options.DomainSuffix is not set
	domainSuffix := "coder"
	if t.options != nil && t.options.DomainSuffix != "" {
		domainSuffix = t.options.DomainSuffix
	}
	return &dnsManager{
		tunnel: t,
		domainSuffix: domainSuffix,
	}
}

type dnsManager struct {
	tunnel *Tunnel
	domainSuffix string
}

func (v *dnsManager) SetDNS(cfg dns.OSConfig) error {
	settings := convertDNSConfig(cfg, v.domainSuffix)
	return v.tunnel.ApplyNetworkSettings(v.tunnel.ctx, &NetworkSettingsRequest{
		DnsSettings: settings,
	})
}

func (*dnsManager) GetBaseConfig() (dns.OSConfig, error) {
	// Tailscale calls this function to blend the OS's DNS configuration with
	// it's own, so this is only called if `SupportsSplitDNS` returns false.
	return dns.OSConfig{}, dns.ErrGetBaseConfigNotSupported
}

func (*dnsManager) SupportsSplitDNS() bool {
	// macOS & Windows 10+ support split DNS, so we'll assume all CoderVPN
	// clients do too.
	return true
}

// Close implements dns.OSConfigurator.
func (*dnsManager) Close() error {
	// There's no cleanup that we need to initiate from within the dylib.
	return nil
}

func convertDNSConfig(cfg dns.OSConfig, domainSuffix string) *NetworkSettingsRequest_DNSSettings {
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
	return &NetworkSettingsRequest_DNSSettings{
		Servers:              servers,
		SearchDomains:        searchDomains,
		DomainName:           domainSuffix,
		MatchDomains:         matchDomains,
		MatchDomainsNoSearch: false,
	}
}
