//go:build !windows

package sessionstore

const defaultServiceName = "not-implemented"

type operatingSystemKeyring struct{}

func (operatingSystemKeyring) Set(_, _ string) error {
	return ErrNotImplemented
}

func (operatingSystemKeyring) Get(_ string) ([]byte, error) {
	return nil, ErrNotImplemented
}

func (operatingSystemKeyring) Delete(_ string) error {
	return ErrNotImplemented
}
