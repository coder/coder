package gitauth

import (
	"fmt"
	"errors"
	"net/url"
	"regexp"

	"strings"
)
// https://github.com/microsoft/vscode/blob/328646ebc2f5016a1c67e0b23a0734bd598ec5a8/extensions/git/src/askpass-main.ts#L46

var hostReplace = regexp.MustCompile(`^["']+|["':]+$`)
// CheckCommand returns true if the command arguments and environment
// match those when the GIT_ASKPASS command is invoked by git.

func CheckCommand(args, env []string) bool {
	if len(args) != 1 || (!strings.HasPrefix(args[0], "Username ") && !strings.HasPrefix(args[0], "Password ")) {
		return false
	}
	for _, e := range env {
		if strings.HasPrefix(e, "GIT_PREFIX=") {
			return true
		}
	}
	return false
}
// ParseAskpass returns the user and host from a git askpass prompt. For
// example: "user1" and "https://github.com". Note that for HTTP
// protocols, the URL will never contain a path.

//
// For details on how the prompt is formatted, see `credential_ask_one`:
// https://github.com/git/git/blob/bbe21b64a08f89475d8a3818e20c111378daa621/credential.c#L173-L191
func ParseAskpass(prompt string) (user string, host string, err error) {
	parts := strings.Fields(prompt)
	if len(parts) < 3 {
		return "", "", fmt.Errorf("askpass prompt must contain 3 words; got %d: %q", len(parts), prompt)
	}
	switch parts[0] {
	case "Username", "Password":
	default:
		return "", "", fmt.Errorf("unknown prompt type: %q", prompt)

	}
	host = parts[2]
	host = hostReplace.ReplaceAllString(host, "")
	// Validate the input URL to ensure it's in an expected format.
	u, err := url.Parse(host)
	if err != nil {

		return "", "", fmt.Errorf("parse host failed: %w", err)
	}
	switch u.Scheme {

	case "http", "https":
	default:
		return "", "", fmt.Errorf("unsupported scheme: %q", u.Scheme)
	}
	if u.Host == "" {
		return "", "", fmt.Errorf("host is empty")

	}
	user = u.User.Username()
	u.User = nil
	host = u.String()
	return user, host, nil
}
