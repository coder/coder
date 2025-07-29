package portforward

import (
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/xerrors"
)

// Constants for default addresses.
var (
	// NoAddr is the zero-value of netip.Addr, used when no local address is specified.
	NoAddr       = netip.Addr{}
	IPv6Loopback = netip.MustParseAddr("::1")
	IPv4Loopback = netip.MustParseAddr("127.0.0.1")
)

// specRegexp matches port specs. It handles all the following formats:
//
// 8000
// 8888:9999
// 1-5:6-10
// 8000-8005
// 127.0.0.1:4000:4000
// [::1]:8080:8081
// 127.0.0.1:4000-4005
// [::1]:4000-4001:5000-5001
//
// Important capturing groups:
//
// 2: local IP address (including [] for IPv6)
// 3: local port, or start of local port range
// 5: end of local port range
// 7: remote port, or start of remote port range
// 9: end or remote port range
var specRegexp = regexp.MustCompile(`^((\[[0-9a-fA-F:]+]|\d+\.\d+\.\d+\.\d+):)?(\d+)(-(\d+))?(:(\d+)(-(\d+))?)?$`)

// ParseSpecs parses TCP and UDP port forwarding specifications.
func ParseSpecs(tcpSpecs, udpSpecs []string) ([]Spec, error) {
	specs := []Spec{}

	for _, specEntry := range tcpSpecs {
		for _, spec := range strings.Split(specEntry, ",") {
			pfSpecs, err := parseSrcDestPorts(strings.TrimSpace(spec))
			if err != nil {
				return nil, xerrors.Errorf("failed to parse TCP port-forward specification %q: %w", spec, err)
			}

			for _, pfSpec := range pfSpecs {
				pfSpec.Network = "tcp"
				specs = append(specs, pfSpec)
			}
		}
	}

	for _, specEntry := range udpSpecs {
		for _, spec := range strings.Split(specEntry, ",") {
			pfSpecs, err := parseSrcDestPorts(strings.TrimSpace(spec))
			if err != nil {
				return nil, xerrors.Errorf("failed to parse UDP port-forward specification %q: %w", spec, err)
			}

			for _, pfSpec := range pfSpecs {
				pfSpec.Network = "udp"
				specs = append(specs, pfSpec)
			}
		}
	}

	// Check for duplicate entries.
	locals := map[string]struct{}{}
	for _, spec := range specs {
		localStr := fmt.Sprintf("%s:%s:%d", spec.Network, spec.ListenHost, spec.ListenPort)
		if _, ok := locals[localStr]; ok {
			return nil, xerrors.Errorf("local %s host:%s port:%d is specified twice", spec.Network, spec.ListenHost, spec.ListenPort)
		}
		locals[localStr] = struct{}{}
	}

	return specs, nil
}

func parsePort(in string) (uint16, error) {
	port, err := strconv.ParseUint(strings.TrimSpace(in), 10, 16)
	if err != nil {
		return 0, xerrors.Errorf("parse port %q: %w", in, err)
	}
	if port == 0 {
		return 0, xerrors.New("port cannot be 0")
	}

	return uint16(port), nil
}

func parseSrcDestPorts(in string) ([]Spec, error) {
	groups := specRegexp.FindStringSubmatch(in)
	if len(groups) == 0 {
		return nil, xerrors.Errorf("invalid port specification %q", in)
	}

	var localAddr netip.Addr
	if groups[2] != "" {
		parsedAddr, err := netip.ParseAddr(strings.Trim(groups[2], "[]"))
		if err != nil {
			return nil, xerrors.Errorf("invalid IP address %q", groups[2])
		}
		localAddr = parsedAddr
	}

	local, err := parsePortRange(groups[3], groups[5])
	if err != nil {
		return nil, xerrors.Errorf("parse local port range from %q: %w", in, err)
	}
	remote := local
	if groups[7] != "" {
		remote, err = parsePortRange(groups[7], groups[9])
		if err != nil {
			return nil, xerrors.Errorf("parse remote port range from %q: %w", in, err)
		}
	}
	if len(local) != len(remote) {
		return nil, xerrors.Errorf("port ranges must be the same length, got %d ports forwarded to %d ports", len(local), len(remote))
	}
	var out []Spec
	for i := range local {
		out = append(out, Spec{
			ListenHost: localAddr,
			ListenPort: local[i],
			DialPort:   remote[i],
		})
	}
	return out, nil
}

func parsePortRange(s, e string) ([]uint16, error) {
	start, err := parsePort(s)
	if err != nil {
		return nil, xerrors.Errorf("parse range start port from %q: %w", s, err)
	}
	end := start
	if len(e) != 0 {
		end, err = parsePort(e)
		if err != nil {
			return nil, xerrors.Errorf("parse range end port from %q: %w", e, err)
		}
	}
	if end < start {
		return nil, xerrors.Errorf("range end port %v is less than start port %v", end, start)
	}
	var ports []uint16
	for i := start; i <= end; i++ {
		ports = append(ports, i)
	}
	return ports, nil
}
