//go:build linux

package nsjail

import (
	"os/exec"

	"golang.org/x/sys/unix"
)

// SetupChildNetworking configures networking within the target process's network
// namespace. This runs inside the child process after it has been
// created and moved to its own network namespace.
func SetupChildNetworking(vethNetJail string) error {
	runner := newCommandRunner([]*command{
		// Assign an IP address to the jail-side veth interface. The /24 mask
		// matches the subnet defined on the host side (192.168.100.0/24),
		// ensuring both interfaces appear on the same L2 network. This address
		// (192.168.100.2) will serve as the jail's primary outbound source IP.
		newCommand(
			"Assign IP to jail-side veth",
			exec.Command("ip", "addr", "add", "192.168.100.2/24", "dev", vethNetJail),
			[]uintptr{uintptr(unix.CAP_NET_ADMIN)},
		),
		// Bring the jail-side veth interface up. Until the interface is set UP,
		// the jail cannot send or receive any packets on this link, even if the
		// IP address and routes are configured correctly.
		newCommand(
			"Activate jail-side veth interface",
			exec.Command("ip", "link", "set", vethNetJail, "up"),
			[]uintptr{uintptr(unix.CAP_NET_ADMIN)},
		),
		// Bring the jail-side veth interface up. Until the interface is set UP,
		// the jail cannot send or receive any packets on this link, even if the
		// IP address and routes are configured correctly.
		newCommand(
			"Enable loopback interface in jail",
			exec.Command("ip", "link", "set", "lo", "up"),
			[]uintptr{uintptr(unix.CAP_NET_ADMIN)},
		),
		// Set the default route for all outbound traffic inside the jail. The
		// gateway is the host-side veth address (192.168.100.1), which performs
		// NAT and transparent TCP interception. This ensures that packets not
		// destined for the jail subnet are routed to the host for processing.
		newCommand(
			"Configure default gateway for jail",
			exec.Command("ip", "route", "add", "default", "via", "192.168.100.1"),
			[]uintptr{uintptr(unix.CAP_NET_ADMIN)},
		),
	})
	if err := runner.run(); err != nil {
		return err
	}

	return nil
}
