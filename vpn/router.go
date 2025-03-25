package vpn

import (
	"net"
	"net/netip"

	"tailscale.com/wgengine/router"
)

func NewRouter(t *Tunnel) router.Router {
	return &vpnRouter{tunnel: t}
}

type vpnRouter struct {
	tunnel *Tunnel
}

func (*vpnRouter) Up() error {
	// On macOS, the Desktop app will handle turning the VPN on and off.
	// On Windows, this is a no-op.
	return nil
}

func (v *vpnRouter) Set(cfg *router.Config) error {
	if cfg == nil {
		return nil
	}
	req := convertRouterConfig(*cfg)
	return v.tunnel.ApplyNetworkSettings(v.tunnel.ctx, req)
}

func (*vpnRouter) Close() error {
	// There's no cleanup that we need to initiate from within the dylib.
	return nil
}

func convertRouterConfig(cfg router.Config) *NetworkSettingsRequest {
	v4LocalAddrs := make([]string, 0)
	v4SubnetMasks := make([]string, 0)
	v6LocalAddrs := make([]string, 0)
	v6PrefixLengths := make([]uint32, 0)
	for _, addrs := range cfg.LocalAddrs {
		switch {
		case addrs.Addr().Is4():
			v4LocalAddrs = append(v4LocalAddrs, addrs.Addr().String())
			v4SubnetMasks = append(v4SubnetMasks, prefixToSubnetMask(addrs))
		case addrs.Addr().Is6():
			v6LocalAddrs = append(v6LocalAddrs, addrs.Addr().String())
			// #nosec G115 - Safe conversion as IPv6 prefix lengths are always within uint32 range (0-128)
			v6PrefixLengths = append(v6PrefixLengths, uint32(addrs.Bits()))
		default:
			continue
		}
	}
	v4Routes := make([]*NetworkSettingsRequest_IPv4Settings_IPv4Route, 0)
	v6Routes := make([]*NetworkSettingsRequest_IPv6Settings_IPv6Route, 0)
	for _, route := range cfg.Routes {
		switch {
		case route.Addr().Is4():
			v4Routes = append(v4Routes, convertToIPV4Route(route))
		case route.Addr().Is6():
			v6Routes = append(v6Routes, convertToIPV6Route(route))
		default:
			continue
		}
	}
	v4ExcludedRoutes := make([]*NetworkSettingsRequest_IPv4Settings_IPv4Route, 0)
	v6ExcludedRoutes := make([]*NetworkSettingsRequest_IPv6Settings_IPv6Route, 0)
	for _, route := range cfg.LocalRoutes {
		switch {
		case route.Addr().Is4():
			v4ExcludedRoutes = append(v4ExcludedRoutes, convertToIPV4Route(route))
		case route.Addr().Is6():
			v6ExcludedRoutes = append(v6ExcludedRoutes, convertToIPV6Route(route))
		default:
			continue
		}
	}

	var v4Settings *NetworkSettingsRequest_IPv4Settings
	if len(v4LocalAddrs) > 0 || len(v4Routes) > 0 || len(v4ExcludedRoutes) > 0 {
		v4Settings = &NetworkSettingsRequest_IPv4Settings{
			Addrs:          v4LocalAddrs,
			SubnetMasks:    v4SubnetMasks,
			IncludedRoutes: v4Routes,
			ExcludedRoutes: v4ExcludedRoutes,
			Router:         "", // NA
		}
	}

	var v6Settings *NetworkSettingsRequest_IPv6Settings
	if len(v6LocalAddrs) > 0 || len(v6Routes) > 0 || len(v6ExcludedRoutes) > 0 {
		v6Settings = &NetworkSettingsRequest_IPv6Settings{
			Addrs:          v6LocalAddrs,
			PrefixLengths:  v6PrefixLengths,
			IncludedRoutes: v6Routes,
			ExcludedRoutes: v6ExcludedRoutes,
		}
	}

	return &NetworkSettingsRequest{
		// #nosec G115 - Safe conversion as MTU values are expected to be small positive integers
		Mtu:                 uint32(cfg.NewMTU),
		Ipv4Settings:        v4Settings,
		Ipv6Settings:        v6Settings,
		TunnelOverheadBytes: 0,  // N/A
		TunnelRemoteAddress: "", // N/A
	}
}

func convertToIPV4Route(route netip.Prefix) *NetworkSettingsRequest_IPv4Settings_IPv4Route {
	return &NetworkSettingsRequest_IPv4Settings_IPv4Route{
		Destination: route.Addr().String(),
		Mask:        prefixToSubnetMask(route),
		Router:      "", // N/A
	}
}

func convertToIPV6Route(route netip.Prefix) *NetworkSettingsRequest_IPv6Settings_IPv6Route {
	return &NetworkSettingsRequest_IPv6Settings_IPv6Route{
		Destination: route.Addr().String(),
		// #nosec G115 - Safe conversion as prefix lengths are always within uint32 range (0-128)
		PrefixLength: uint32(route.Bits()),
		Router:       "", // N/A
	}
}

func prefixToSubnetMask(prefix netip.Prefix) string {
	maskBytes := net.CIDRMask(prefix.Masked().Bits(), net.IPv4len*8)
	return net.IP(maskBytes).String()
}