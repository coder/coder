//go:build windows

package agentsocket

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/user"
	"strings"

	"github.com/Microsoft/go-winio"
	"golang.org/x/xerrors"
)

const defaultSocketPath = `\\.\pipe\com.coder.agentsocket`

func createSocket(path string) (net.Listener, error) {
	if path == "" {
		path = defaultSocketPath
	}
	if !strings.HasPrefix(path, `\\.\pipe\`) {
		return nil, xerrors.Errorf("%q is not a valid local socket path", path)
	}

	user, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("unable to look up current user: %w", err)
	}
	sid := user.Uid

	// SecurityDescriptor is in SDDL format. c.f.
	// https://learn.microsoft.com/en-us/windows/win32/secauthz/security-descriptor-string-format for full details.
	// D: indicates this is a Discretionary Access Control List (DACL), which is Windows-speak for ACLs that allow or
	// deny access (as opposed to SACL which controls audit logging).
	// P indicates that this DACL is "protected" from being modified thru inheritance
	// () delimit access control entries (ACEs), here we only have one, which, allows (A) generic all (GA) access to our
	// specific user's security ID (SID).
	//
	// Note that although Microsoft docs at https://learn.microsoft.com/en-us/windows/win32/ipc/named-pipes warns that
	// named pipes are accessible from remote machines in the general case, the `winio` package sets the flag
	// windows.FILE_PIPE_REJECT_REMOTE_CLIENTS when creating pipes, so connections from remote machines are always
	// denied. This is important because we sort of expect customers to run the Coder agent under a generic user
	// account unless they are very sophisticated. We don't want this socket to cross the boundary of the local machine.
	configuration := &winio.PipeConfig{
		SecurityDescriptor: fmt.Sprintf("D:P(A;;GA;;;%s)", sid),
	}

	listener, err := winio.ListenPipe(path, configuration)
	if err != nil {
		return nil, xerrors.Errorf("failed to open named pipe: %w", err)
	}
	return listener, nil
}

func cleanupSocket(path string) error {
	return os.Remove(path)
}

func dialSocket(ctx context.Context, path string) (net.Conn, error) {
	return winio.DialPipeContext(ctx, path)
}
