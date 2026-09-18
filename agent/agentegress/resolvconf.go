package agentegress

import (
	"bufio"
	"io"
	"net/netip"
	"os"
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
	allLoopback := len(resolvers) > 0
	for _, addr := range resolvers {
		if !addr.IsLoopback() {
			allLoopback = false
			break
		}
	}
	if allLoopback {
		if upstream := readResolvConf(systemdResolvConfPath); len(upstream) > 0 {
			return upstream
		}
	}
	return resolvers
}

func readResolvConf(path string) []netip.Addr {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	return parseResolvConf(f)
}

// parseResolvConf extracts nameserver addresses in file order. Only the
// nameserver directive matters here: search domains and options are the
// libc resolver's business, and the proxy forwards queries verbatim.
func parseResolvConf(r io.Reader) []netip.Addr {
	var out []netip.Addr
	seen := make(map[netip.Addr]struct{})
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexAny(line, "#;"); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "nameserver" {
			continue
		}
		// Scoped IPv6 literals (fe80::1%eth0) carry a zone that iptables
		// and the dialer do not want.
		host, _, _ := strings.Cut(fields[1], "%")
		addr, err := netip.ParseAddr(host)
		if err != nil {
			continue
		}
		addr = addr.Unmap()
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		out = append(out, addr)
	}
	return out
}
