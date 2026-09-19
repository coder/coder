//go:build !linux

package agentegress

import "errors"

func lockdownNetAdmin() error {
	return errors.ErrUnsupported
}
