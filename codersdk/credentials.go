package codersdk

import (
	"net/http"

	"github.com/coder/websocket"
)

// SessionTokenProvider provides the session token to access the Coder service (coderd).
// @typescript-ignore SessionTokenProvider
type SessionTokenProvider interface {
	// AsRequestOption returns a request option that attaches the session token to an HTTP request.
	AsRequestOption() RequestOption
	// SetDialOption sets the session token on a websocket request via DialOptions
	SetDialOption(options *websocket.DialOptions)
	// GetSessionToken returns the session token as a string.
	GetSessionToken() string
}

// FixedSessionTokenProvider provides a given, fixed, session token. E.g. one read from file or environment variable
// at the program start.
// @typescript-ignore FixedSessionTokenProvider
type FixedSessionTokenProvider struct {
	SessionToken string
	// SessionTokenHeader is an optional custom header to use for setting tokens. By
	// default, 'Coder-Session-Token' is used.
	SessionTokenHeader string
}

func (f FixedSessionTokenProvider) AsRequestOption() RequestOption {
	return func(req *http.Request) {
		tokenHeader := f.SessionTokenHeader
		if tokenHeader == "" {
			tokenHeader = SessionTokenHeader
		}
		req.Header.Set(tokenHeader, f.SessionToken)
	}
}

func (f FixedSessionTokenProvider) GetSessionToken() string {
	return f.SessionToken
}

func (f FixedSessionTokenProvider) SetDialOption(opts *websocket.DialOptions) {
	tokenHeader := f.SessionTokenHeader
	if tokenHeader == "" {
		tokenHeader = SessionTokenHeader
	}
	if opts.HTTPHeader == nil {
		opts.HTTPHeader = http.Header{}
	}
	if opts.HTTPHeader.Get(tokenHeader) == "" {
		opts.HTTPHeader.Set(tokenHeader, f.SessionToken)
	}
}
