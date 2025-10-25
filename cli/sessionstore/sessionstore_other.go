//go:build !windows

package sessionstore

const serviceName = "not-implemented"

type operatingSystemKeyring struct{}

func (operatingSystemKeyring) Set(_, _ string) error {
	return ErrNotImplemented
}

func (operatingSystemKeyring) Get(_ string) (string, error) {
	return "", ErrNotImplemented
}

func (operatingSystemKeyring) Delete(_ string) error {
	return ErrNotImplemented
}
