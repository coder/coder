//go:build !darwin && !windows

package vpn

import "cdr.dev/slog"

// This is a no-op on every platform except Darwin and Windows.
func GetNetworkingStack(_ *Tunnel, _ *StartRequest, _ slog.Logger) (NetworkStack, error) {
	return NetworkStack{}, nil
}
