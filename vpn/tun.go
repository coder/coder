//go:build !darwin && !windows && !linux

package vpn

import "cdr.dev/slog/v3"

// This is a no-op on every platform except Darwin, Windows, and Linux.
func GetNetworkingStack(_ *Tunnel, _ *StartRequest, _ slog.Logger) (NetworkStack, error) {
	return NetworkStack{}, nil
}
