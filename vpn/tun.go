//go:build !darwin && !windows

package vpn

import "cdr.dev/slog"

// This is a no-op on every platform except Darwin and Windows.
func GetNetworkingStack(t *Tunnel, req *StartRequest, logger slog.Logger) (NetworkStack, error) {
	return NetworkStack{}, nil
}
