//go:build !linux && !windows
// +build !linux,!windows

package agent

import "github.com/coder/coder/codersdk"

func (lp *listeningPortsHandler) getListeningPorts() ([]codersdk.ListeningPort, error) {
	// Can't scan for ports on non-linux or non-windows systems at the moment.
	// The UI will not show any "no ports found" message to the user, so the
	// user won't suspect a thing.
	return []codersdk.ListeningPort{}, nil
}
