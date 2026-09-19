//go:build !linux

package agentegress

import "errors"

func lockdownNetworkCapabilities() error {
	return errors.ErrUnsupported
}
