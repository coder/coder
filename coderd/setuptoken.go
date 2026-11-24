package coderd

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"

	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/codersdk"
)

// setupTokenRequired reports whether the first user can only be
// created with the setup token.
func (api *API) setupTokenRequired() bool {
	return api.SetupToken != ""
}

// setupTokenMatches compares digests so that the time taken does not
// depend on the length of got or on where it first differs.
func (api *API) setupTokenMatches(got string) bool {
	want := sha256.Sum256([]byte(api.SetupToken))
	have := sha256.Sum256([]byte(got))
	return subtle.ConstantTimeCompare(want[:], have[:]) == 1
}

// setupTokenRefused is the one response for a missing or wrong token,
// so a caller cannot learn how the token was wrong.
func setupTokenRefused() codersdk.Response {
	return codersdk.Response{
		Message: "A setup token is required to create the first user. Find it in the output of coder server.",
		Validations: []codersdk.ValidationError{{
			Field:  "setup_token",
			Detail: "Missing or incorrect setup token.",
		}},
	}
}

// errSetupNotComplete refuses an OAuth sign-up of the first user.
func errSetupNotComplete() *idpsync.HTTPError {
	return &idpsync.HTTPError{
		Code:             http.StatusForbidden,
		Msg:              "Setup is not complete",
		Detail:           "Create the first user with the setup token from the output of coder server. You can sign in after that.",
		RenderStaticPage: true,
	}
}
