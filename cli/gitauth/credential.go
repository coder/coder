package gitauth

import (
	"bufio"
	"io"
	"net/url"
	"strings"

	"golang.org/x/xerrors"
)

// CredentialRequest is the subset of a Git credential helper request that
// Coder uses to pick an external auth provider. Git writes the request to the
// helper's stdin as "key=value" lines terminated by a blank line or EOF.
//
// See https://git-scm.com/docs/git-credential#IOFMT for the format.
type CredentialRequest struct {
	Protocol string
	Host     string
	// Path is only sent when credential.useHttpPath is enabled.
	Path string
}

// ParseCredentialRequest reads a Git credential helper request from r.
// Unknown attributes are ignored.
func ParseCredentialRequest(r io.Reader) (CredentialRequest, error) {
	var req CredentialRequest
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			break
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "protocol":
			req.Protocol = value
		case "host":
			req.Host = value
		case "path":
			req.Path = value
		}
	}
	if err := scanner.Err(); err != nil {
		return CredentialRequest{}, xerrors.Errorf("read credential request: %w", err)
	}
	return req, nil
}

// MatchURL returns the URL that Coder matches against external auth provider
// regexes. It has the same shape as the URL that GIT_ASKPASS receives, so
// both mechanisms resolve the same provider. ok is false for protocols that
// external auth does not serve, such as ssh.
func (r CredentialRequest) MatchURL() (match string, ok bool) {
	switch r.Protocol {
	case "http", "https":
	default:
		return "", false
	}
	if r.Host == "" {
		return "", false
	}
	u := url.URL{Scheme: r.Protocol, Host: r.Host}
	if r.Path != "" {
		u.Path = "/" + strings.TrimPrefix(r.Path, "/")
	}
	return u.String(), true
}
