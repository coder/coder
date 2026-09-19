//go:build !linux

package agentegress

import "errors"

func dropCapabilityBoundingSet(uintptr, uintptr, uintptr, uintptr, uintptr) error {
	return errors.ErrUnsupported
}

func dropNetAdmin(func(uintptr, uintptr, uintptr, uintptr, uintptr) error) error {
	return errors.ErrUnsupported
}
