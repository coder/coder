//go:build !windows

package cli

import (
	"strings"

	"golang.org/x/xerrors"
)

var hideForceUnixSlashes = true

// sshConfigMatchExecEscape prepares the path for use in `Match exec` statement.
//
// OpenSSH parses the Match line with a very simple tokenizer that accepts "-enclosed strings for the exec command, and
// has no supported escape sequences for ". This means we cannot include " within the command to execute.
func sshConfigMatchExecEscape(path string) (string, error) {
	// This is unlikely to ever happen, but newlines are allowed on
	// certain filesystems, but cannot be used inside ssh config.
	if strings.ContainsAny(path, "\n") {
		return "", xerrors.Errorf("invalid path: %s", path)
	}
	// Quotes are allowed in path names on unix-like file systems, but OpenSSH's parsing of `Match exec` doesn't allow
	// them.
	if strings.Contains(path, `"`) {
		return "", xerrors.Errorf("path must not contain quotes: %q", path)
	}

	// OpenSSH passes the match exec string directly to the user's shell. sh, bash and zsh accept spaces, tabs and
	// backslashes simply escaped by a `\`. It's hard to predict exactly what more exotic shells might do, but this
	// should work for macOS and most Linux distros in their default configuration.
	path = strings.ReplaceAll(path, `\`, `\\`) // must be first, since later replacements add backslashes.
	path = strings.ReplaceAll(path, " ", "\\ ")
	path = strings.ReplaceAll(path, "\t", "\\\t")
	return path, nil
}
