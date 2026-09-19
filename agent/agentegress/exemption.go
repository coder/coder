package agentegress

import (
	"net"
	"strconv"
	"strings"

	"golang.org/x/xerrors"
)

// Exemption identifies one protocol-specific control plane destination.
type Exemption struct {
	Proto string
	Host  string
	Port  uint16
}

// ParseExemption parses a control plane exemption in proto/host:port format.
// Protocol and port are mandatory so malformed entries never become wildcard
// exemptions.
func ParseExemption(value string) (Exemption, error) {
	proto, hostport, ok := strings.Cut(strings.TrimSpace(value), "/")
	if !ok || (proto != "tcp" && proto != "udp") {
		return Exemption{}, xerrors.Errorf("invalid exemption protocol in %q", value)
	}
	host, portString, err := net.SplitHostPort(hostport)
	if err != nil {
		return Exemption{}, xerrors.Errorf("parse exemption address %q: %w", value, err)
	}
	if host == "" {
		return Exemption{}, xerrors.Errorf("exemption host is required in %q", value)
	}
	port, err := strconv.ParseUint(portString, 10, 16)
	if err != nil || port == 0 {
		return Exemption{}, xerrors.Errorf("invalid exemption port in %q", value)
	}
	return Exemption{Proto: proto, Host: host, Port: uint16(port)}, nil
}
