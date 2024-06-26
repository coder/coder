package healthsdk

import (
	"net"

	"tailscale.com/net/interfaces"

	"github.com/coder/coder/v2/coderd/healthcheck/health"
)

// gVisor is nominally permitted to send packets up to 1280.
// Wireguard adds 30 bytes (1310)
// UDP adds 8 bytes (1318)
// IP adds 20-60 bytes (1338-1378)
// So, it really needs to be 1378 to be totally safe
const safeMTU = 1378

// @typescript-ignore InterfacesReport
type InterfacesReport struct {
	BaseReport
	Interfaces []Interface `json:"interfaces"`
}

// @typescript-ignore Interface
type Interface struct {
	Name      string   `json:"name"`
	MTU       int      `json:"mtu"`
	Addresses []string `json:"addresses"`
}

func RunInterfacesReport() (InterfacesReport, error) {
	st, err := interfaces.GetState()
	if err != nil {
		return InterfacesReport{}, err
	}
	return generateInterfacesReport(st), nil
}

func generateInterfacesReport(st *interfaces.State) (report InterfacesReport) {
	report.Severity = health.SeverityOK
	for name, iface := range st.Interface {
		// macOS has a ton of random interfaces, so to keep things helpful, let's filter out any
		// that:
		//
		// - are not enabled
		// - don't have any addresses
		// - have only link-local addresses (e.g. fe80:...)
		if (iface.Flags & net.FlagUp) == 0 {
			continue
		}
		addrs := st.InterfaceIPs[name]
		if len(addrs) == 0 {
			continue
		}
		var r bool
		healthIface := Interface{
			Name: iface.Name,
			MTU:  iface.MTU,
		}
		for _, addr := range addrs {
			healthIface.Addresses = append(healthIface.Addresses, addr.String())
			if addr.Addr().IsLinkLocalUnicast() || addr.Addr().IsLinkLocalMulticast() {
				continue
			}
			r = true
		}
		if !r {
			continue
		}
		report.Interfaces = append(report.Interfaces, healthIface)
		if iface.MTU < safeMTU {
			report.Severity = health.SeverityWarning
			report.Warnings = append(report.Warnings,
				health.Messagef(health.CodeInterfaceSmallMTU,
					"network interface %s has MTU %d (less than %d), which may cause problems with direct connections", iface.Name, iface.MTU, safeMTU),
			)
		}
	}
	return report
}
