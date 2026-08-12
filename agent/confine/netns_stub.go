//go:build !linux

package confine

import (
	"context"

	"golang.org/x/xerrors"
)

// NetworkNamespaceOptions controls creation of a confined network namespace.
type NetworkNamespaceOptions struct {
	Pool string
}

// NetworkNamespacePorts identifies the host-side egress listeners.
type NetworkNamespacePorts struct {
	HTTP uint16
	SNI  uint16
	DNS  uint16
}

// NetworkNamespace is unavailable on non-Linux systems.
type NetworkNamespace struct {
	hostIP string
}

type networkNamespace = NetworkNamespace

// OpenNetworkNamespace reports that network namespaces require Linux.
func OpenNetworkNamespace(context.Context, NetworkNamespaceOptions) (*NetworkNamespace, error) {
	return nil, xerrors.New("network namespace confinement requires Linux")
}

func newNetworkNamespace(ctx context.Context) (*networkNamespace, error) {
	return OpenNetworkNamespace(ctx, NetworkNamespaceOptions{})
}

// HostIP returns an empty address on unsupported systems.
func (*NetworkNamespace) HostIP() string {
	return ""
}

// ConfigureEgress reports that network namespaces require Linux.
func (*NetworkNamespace) ConfigureEgress(context.Context, NetworkNamespacePorts) error {
	return xerrors.New("network namespace confinement requires Linux")
}

// CommandArgs returns args unchanged on unsupported systems.
func (*NetworkNamespace) CommandArgs(args []string) []string {
	return args
}

func (n *NetworkNamespace) execArgs(args []string) []string {
	return n.CommandArgs(args)
}

// Close is a no-op on unsupported systems.
func (*NetworkNamespace) Close() error {
	return nil
}
