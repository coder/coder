//go:build !darwin

package vpn

import "github.com/tailscale/wireguard-go/tun"

// This is a no-op on non-Darwin platforms.
func makeTUN(int) (tun.Device, error) {
	return nil, nil
}
