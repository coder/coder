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
	req := convertRouterConfig(cfg)
	return v.tunnel.ApplyNetworkSettings(v.tunnel.ctx, req)
}

func (*vpnRouter) Close() error {
	// There's no cleanup that we need to initiate from within the dylib.
	return nil
}

func convertRouterConfig(cfg *router.Config) *NetworkSettingsRequest {
	v4LocalAddrs := make([]string, 0)
	v6LocalAddrs := make([]string, 0)
	for _, addrs := range cfg.LocalAddrs {
		if addrs.Addr().Is4() {
			v4LocalAddrs = append(v4LocalAddrs, addrs.String())
		} else if addrs.Addr().Is6() {
			v6LocalAddrs = append(v6LocalAddrs, addrs.String())
		} else {
			continue
		}
	}
	v4Routes := make([]*NetworkSettingsRequest_IPv4Settings_IPv4Route, 0)
	v6Routes := make([]*NetworkSettingsRequest_IPv6Settings_IPv6Route, 0)
	for _, route := range cfg.Routes {
		if route.Addr().Is4() {
			v4Routes = append(v4Routes, convertToIPV4Route(route))
		} else if route.Addr().Is6() {
			v6Routes = append(v6Routes, convertToIPV6Route(route))
		} else {
			continue
		}
	}
	v4ExcludedRoutes := make([]*NetworkSettingsRequest_IPv4Settings_IPv4Route, 0)
	v6ExcludedRoutes := make([]*NetworkSettingsRequest_IPv6Settings_IPv6Route, 0)
	for _, route := range cfg.LocalRoutes {
		if route.Addr().Is4() {
			v4ExcludedRoutes = append(v4ExcludedRoutes, convertToIPV4Route(route))
		} else if route.Addr().Is6() {
			v6ExcludedRoutes = append(v6ExcludedRoutes, convertToIPV6Route(route))
		} else {
			continue
		}
	}

	return &NetworkSettingsRequest{
		Mtu: uint32(cfg.NewMTU),
		Ipv4Settings: &NetworkSettingsRequest_IPv4Settings{
			Addrs:          v4LocalAddrs,
			IncludedRoutes: v4Routes,
			ExcludedRoutes: v4ExcludedRoutes,
		},
		Ipv6Settings: &NetworkSettingsRequest_IPv6Settings{
			Addrs:          v6LocalAddrs,
			IncludedRoutes: v6Routes,
			ExcludedRoutes: v6ExcludedRoutes,
		},
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
		Destination:  route.Addr().String(),
		PrefixLength: uint32(route.Bits()),
		Router:       "", // N/A
	}
}

func prefixToSubnetMask(prefix netip.Prefix) string {
	maskBytes := net.CIDRMask(prefix.Masked().Bits(), net.IPv4len*8)
	return net.IP(maskBytes).String()
}
