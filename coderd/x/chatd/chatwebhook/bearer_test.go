package chatwebhook_test

import (
	"net/http"
	"testing"

	"github.com/coder/coder/v2/coderd/x/chatd/chatwebhook"
)

func TestBearer(t *testing.T) {
	t.Parallel()

	const secret = "q1w2e3r4t5y6u7i8o9p0q1w2e3r4t5y6u7i8o9p0abc"
	auth := func(value string) http.Header { return header("Authorization", value) }

	runVerifyCases(t, newVerifier(t, chatwebhook.SchemeBearer), []verifyCase{
		{name: "valid token", header: auth("Bearer " + secret), secret: secret, accepted: true},
		{name: "scheme word in lower case", header: auth("bearer " + secret), secret: secret, accepted: true},
		// RFC 6750 section 2.1: credentials = "Bearer" 1*SP b64token.
		{name: "two spaces before the token", header: auth("Bearer  " + secret), secret: secret, accepted: true},

		{name: "header missing", header: http.Header{}, secret: secret},
		{name: "wrong scheme Digest", header: auth("Digest " + secret), secret: secret},
		{name: "no space after the scheme", header: auth("Bearer" + secret), secret: secret},
		{name: "scheme only", header: auth("Bearer"), secret: secret},
		{name: "spaces only after the scheme and an empty secret", header: auth("Bearer    "), secret: ""},
		{name: "tab before the token", header: auth("Bearer\t" + secret), secret: secret},
		{name: "tab after the spaces", header: auth("Bearer  \t" + secret), secret: secret},
		{name: "wrong token", header: auth("Bearer not-the-secret"), secret: secret},
		{name: "token is a strict prefix of the secret", header: auth("Bearer " + secret[:len(secret)-1]), secret: secret},
		{name: "secret is a strict prefix of the token", header: auth("Bearer " + secret + "x"), secret: secret},
		{name: "token differs in the last byte", header: auth("Bearer " + secret[:len(secret)-1] + "X"), secret: secret},
		{name: "two Authorization lines, first valid", header: header("Authorization", "Bearer "+secret, "Authorization", "Bearer x"), secret: secret},
		{name: "two Authorization lines, second valid", header: header("Authorization", "Bearer x", "Authorization", "Bearer "+secret), secret: secret},
		{name: "empty secret and empty token", header: auth("Bearer "), secret: ""},
		{name: "empty secret and any token", header: auth("Bearer anything"), secret: ""},
	})
}
