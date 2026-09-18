//go:build !linux

package agentegress

import "context"

// Install is unsupported outside Linux; the caller falls back to advisory
// proxy mode.
func (*Enforcer) Install(context.Context) error {
	return ErrEnforcementUnavailable
}

// Remove is a no-op outside Linux.
func (*Enforcer) Remove(context.Context) error {
	return nil
}
