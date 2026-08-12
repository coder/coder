package confine

import "fmt"

// NetworkNamespaceOptions controls creation of a confined network namespace.
type NetworkNamespaceOptions struct {
	// Pool is an IPv4 prefix divided into /30 networks. An empty value uses
	// DefaultNetNSPool.
	Pool string
}

// NetworkNamespacePorts identifies the host-side egress listeners.
type NetworkNamespacePorts struct {
	HTTP uint16
	SNI  uint16
	DNS  uint16
}

// NetworkNamespaceUnsupportedError reports why host network namespace
// confinement cannot be enforced.
type NetworkNamespaceUnsupportedError struct {
	Reason string
	Err    error
}

func (e *NetworkNamespaceUnsupportedError) Error() string {
	if e.Err == nil {
		return "unsupported: " + e.Reason
	}
	return fmt.Sprintf("unsupported: %s: %v", e.Reason, e.Err)
}

// Unwrap returns the error that caused the preflight failure.
func (e *NetworkNamespaceUnsupportedError) Unwrap() error {
	return e.Err
}

func unsupportedNetworkNamespace(reason string, err error) error {
	return &NetworkNamespaceUnsupportedError{Reason: reason, Err: err}
}
