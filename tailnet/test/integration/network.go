//go:build linux
// +build linux

package integration

import (
	"fmt"
	"os"
	"os/exec"

	"golang.org/x/xerrors"
)

// createNetNS creates a new network namespace with the given name. The returned
// file is a file descriptor to the network namespace.
func createNetNS(name string) (*os.File, error) {
	// We use ip-netns here because it handles the process of creating a
	// disowned netns for us.
	// The only way to create a network namespace is by calling unshare(2) or
	// clone(2) with the CLONE_NEWNET flag, and as soon as the last process in a
	// network namespace exits, the namespace is destroyed.
	// However, if you create a bind mount of /proc/$PID/ns/net to a file, it
	// will keep the namespace alive until the mount is removed.
	// ip-netns does this for us. Without it, we would have to fork anyways.
	// Later, we will use nsenter to enter this network namespace.
	err := exec.Command("ip", "netns", "add", name).Run()
	if err != nil {
		return nil, xerrors.Errorf("create network namespace via ip-netns: %w", err)
	}

	// Open /run/netns/$name to get a file descriptor to the network namespace
	// so it stays active after we soft-delete it.
	path := fmt.Sprintf("/run/netns/%s", name)
	file, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, xerrors.Errorf("open network namespace file %q: %w", path, err)
	}

	// Exec "ip link set lo up" in the namespace to bring up loopback
	// networking.
	//nolint:gosec
	err = exec.Command("ip", "netns", "exec", name, "ip", "link", "set", "lo", "up").Run()
	if err != nil {
		return nil, xerrors.Errorf("bring up loopback interface in network namespace: %w", err)
	}

	// Remove the network namespace. The kernel will keep it around until the
	// file descriptor is closed.
	err = exec.Command("ip", "netns", "delete", name).Run()
	if err != nil {
		return nil, xerrors.Errorf("soft delete network namespace via ip-netns: %w", err)
	}

	return file, nil
}
