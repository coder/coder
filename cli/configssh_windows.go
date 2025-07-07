//go:build windows

package cli

import (
	"fmt"
	"strings"

	"golang.org/x/xerrors"
)

// Must be a var for unit tests to conform behavior
var hideForceUnixSlashes = false

// sshConfigMatchExecEscape prepares the path for use in `Match exec` statement.
//
// OpenSSH parses the Match line with a very simple tokenizer that accepts "-enclosed strings for the exec command, and
// has no supported escape sequences for ". This means we cannot include " within the command to execute.
//
// To make matters worse, on Windows, OpenSSH passes the string directly to cmd.exe for execution, and as far as I can
// tell, the only supported way to call a path that has spaces in it is to surround it with ".
//
// So, we can't actually include " directly, but here is a horrible workaround:
//
// "for /f %%a in ('powershell.exe -Command [char]34') do @cmd.exe /c %%aC:\Program Files\Coder\bin\coder.exe%%a connect exists %h"
//
// The key insight here is to store the character " in a variable (%a in this case, but the % itself needs to be
// escaped, so it becomes %%a), and then use that variable to construct the double-quoted path:
//
// %%aC:\Program Files\Coder\bin\coder.exe%%a.
//
// How do we generate a single " character without actually using that character? I couldn't find any command in cmd.exe
// to do it, but powershell.exe can convert ASCII to characters like this: `[char]34` (where 34 is the code point for ").
//
// Other notes:
//   - @ in `@cmd.exe` suppresses echoing it, so you don't get this command printed
//   - we need another invocation of cmd.exe (e.g. `do @cmd.exe /c %%aC:\Program Files\Coder\bin\coder.exe%%a`). Without
//     it the double-quote gets interpreted as part of the path, and you get: '"C:\Program' is not recognized.
//     Constructing the string and then passing it to another instance of cmd.exe does this trick here.
//   - OpenSSH passes the `Match exec` command to cmd.exe regardless of whether the user has a unix-like shell like
//     git bash, so we don't have a `forceUnixPath` option like for the ProxyCommand which does respect the user's
//     configured shell on Windows.
func sshConfigMatchExecEscape(path string) (string, error) {
	// This is unlikely to ever happen, but newlines are allowed on
	// certain filesystems, but cannot be used inside ssh config.
	if strings.ContainsAny(path, "\n") {
		return "", xerrors.Errorf("invalid path: %s", path)
	}
	// Windows does not allow double-quotes or tabs in paths. If we get one it is an error.
	if strings.ContainsAny(path, "\"\t") {
		return "", xerrors.Errorf("path must not contain quotes or tabs: %q", path)
	}

	if strings.ContainsAny(path, " ") {
		// c.f. function comment for how this works.
		path = fmt.Sprintf("for /f %%%%a in ('powershell.exe -Command [char]34') do @cmd.exe /c %%%%a%s%%%%a", path) //nolint:gocritic // We don't want %q here.
	}
	return path, nil
}
