package agentegress

import (
	"net/netip"
	"os"
	"slices"
	"strings"
)

const (
	resolvConfPath = "/etc/resolv.conf"
	// systemdResolvConfPath lists the real upstream servers when
	// /etc/resolv.conf points at the systemd-resolved stub.
	systemdResolvConfPath = "/run/systemd/resolve/resolv.conf"
)

// systemResolvers returns the nameservers the proxy should forward exempt
// queries to. When /etc/resolv.conf names only loopback stubs (such as
// systemd-resolved on 127.0.0.53), the stub's own upstreams are used
// instead: the stub sends UDP, which enforcement would redirect straight
// back into the proxy, and its upstreams are what the enforcer must exempt.
// A missing or unreadable file yields no resolvers.
func systemResolvers() []netip.Addr {
	resolvers := readResolvConf(resolvConfPath)
	allLoopback := len(resolvers) > 0 && !slices.ContainsFunc(resolvers, func(a netip.Addr) bool { return !a.IsLoopback() })
	if allLoopback {
		if upstream := readResolvConf(systemdResolvConfPath); len(upstream) > 0 {
			return upstream
		}
	}
	return resolvers
}

func readResolvConf(path string) []netip.Addr {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return parseResolvConf(string(data))
}

// parseResolvConf extracts nameserver addresses in file order. Only the
// nameserver directive matters here: search domains and options are the
// libc resolver's business, and the proxy forwards queries verbatim.
func parseResolvConf(conf string) []netip.Addr {
	var out []netip.Addr
	for line := range strings.Lines(conf) {
		line, _, _ = strings.Cut(line, "#")
		line, _, _ = strings.Cut(line, ";")
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "nameserver" {
			continue
		}
		// Scoped IPv6 literals (fe80::1%eth0) carry a zone that iptables
		// and the dialer do not want.
		host, _, _ := strings.Cut(fields[1], "%")
		addr, err := netip.ParseAddr(host)
		if err == nil && !slices.Contains(out, addr.Unmap()) {
			out = append(out, addr.Unmap())
		}
	}
	return out
}
