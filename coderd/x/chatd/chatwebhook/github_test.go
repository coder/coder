package chatwebhook_test

import (
	"encoding/hex"
	"net/http"
	"strings"
	"testing"

	"github.com/coder/coder/v2/coderd/x/chatd/chatwebhook"
)

func TestGitHub(t *testing.T) {
	t.Parallel()

	// Official vector from
	// https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries
	const (
		vectorSecret = "It's a Secret to Everybody"
		vectorBody   = "Hello, World!"
		vectorSig    = "sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17"
	)

	const secret = "j9X0c2Vy-Z2l0aHViLXRlc3Qtc2VjcmV0LTAxMjM0NTY" //nolint:gosec // Test fixture, not a credential.
	signHex := func(secret string, body []byte) string {
		return hex.EncodeToString(hmacSHA256([]byte(secret), body))
	}
	sig := func(value string) http.Header { return header("X-Hub-Signature-256", value) }
	body := []byte(`{"action":"opened","number":1}`)
	valid := signHex(secret, body)

	runVerifyCases(t, newVerifier(t, chatwebhook.SchemeGitHub), []verifyCase{
		{name: "official vector", header: sig(vectorSig), body: []byte(vectorBody), secret: vectorSecret, accepted: true},
		{name: "valid signature", header: sig("sha256=" + valid), body: body, secret: secret, accepted: true},

		{name: "header missing", header: http.Header{}, body: body, secret: secret},
		{name: "no prefix", header: sig(valid), body: body, secret: secret},
		{name: "truncated digest", header: sig("sha256=" + valid[:62]), body: body, secret: secret},
		{name: "digest with extra bytes", header: sig("sha256=" + valid + "00"), body: body, secret: secret},
		{name: "two lines, first valid", header: header("X-Hub-Signature-256", "sha256="+valid, "X-Hub-Signature-256", "sha256="+strings.Repeat("0", 64)), body: body, secret: secret},
		{name: "two lines, second valid", header: header("X-Hub-Signature-256", "sha256="+strings.Repeat("0", 64), "X-Hub-Signature-256", "sha256="+valid), body: body, secret: secret},
		{name: "wrong secret", header: sig("sha256=" + valid), body: body, secret: secret + "x"},
		{name: "tampered body", header: sig("sha256=" + valid), body: append([]byte(nil), append(body, ' ')...), secret: secret},
		{name: "signature made with an empty key", header: sig("sha256=" + signHex("", body)), body: body, secret: ""},
	})
}
