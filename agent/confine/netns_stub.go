//go:build !linux

package confine

import "context"

// NetworkNamespace is unavailable on non-Linux systems.
type NetworkNamespace struct {
	hostIP string
}

type networkNamespace = NetworkNamespace

// PreflightNetworkNamespace reports that network namespaces require Linux.
func PreflightNetworkNamespace(context.Context) error {
	return unsupportedNetworkNamespace("network namespace confinement requires Linux", nil)
}

// OpenNetworkNamespace reports that network namespaces require Linux.
func OpenNetworkNamespace(ctx context.Context, _ NetworkNamespaceOptions) (*NetworkNamespace, error) {
	return nil, PreflightNetworkNamespace(ctx)
}

func newNetworkNamespace(ctx context.Context) (*networkNamespace, error) {
	return OpenNetworkNamespace(ctx, NetworkNamespaceOptions{})
}

// HostIP returns an empty address on unsupported systems.
func (*NetworkNamespace) HostIP() string {
	return ""
}

// ConfigureEgress reports that network namespaces require Linux.
func (*NetworkNamespace) ConfigureEgress(ctx context.Context, _ NetworkNamespacePorts) error {
	return PreflightNetworkNamespace(ctx)
}

// CommandArgs reports that network namespaces require Linux.
func (*NetworkNamespace) CommandArgs([]string) ([]string, error) {
	return nil, unsupportedNetworkNamespace("network namespace confinement requires Linux", nil)
}

func (n *NetworkNamespace) execArgs(args []string) []string {
	result, err := n.CommandArgs(args)
	if err != nil {
		return []string{"false"}
	}
	return result
}

// Close is a no-op on unsupported systems.
func (*NetworkNamespace) Close() error {
	return nil
}
