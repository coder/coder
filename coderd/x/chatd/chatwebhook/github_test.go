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
		{name: "upper case hex digest", header: sig("sha256=" + strings.ToUpper(valid)), body: body, secret: secret, accepted: true},
		{name: "empty body signed", header: sig("sha256=" + signHex(secret, nil)), body: nil, secret: secret, accepted: true},
		{name: "non-ASCII body signed", header: sig("sha256=" + signHex(secret, []byte("héllo wörld ✓"))), body: []byte("héllo wörld ✓"), secret: secret, accepted: true},
		{name: "invalid UTF-8 body signed", header: sig("sha256=" + signHex(secret, []byte{0xff, 0xfe, 0x00, 0x80})), body: []byte{0xff, 0xfe, 0x00, 0x80}, secret: secret, accepted: true},
		{name: "non-ASCII secret", header: sig("sha256=" + signHex("sécrét-✓", body)), body: body, secret: "sécrét-✓", accepted: true},

		{name: "header missing", header: http.Header{}, body: body, secret: secret},
		{name: "header nil", header: nil, body: body, secret: secret},
		{name: "legacy sha1 header only", header: header("X-Hub-Signature", "sha1="+valid[:40]), body: body, secret: secret},
		{name: "empty header value", header: sig(""), body: body, secret: secret},
		{name: "prefix only", header: sig("sha256="), body: body, secret: secret},
		{name: "no prefix", header: sig(valid), body: body, secret: secret},
		{name: "wrong prefix sha1", header: sig("sha1=" + valid), body: body, secret: secret},
		{name: "wrong separator", header: sig("sha256:" + valid), body: body, secret: secret},
		{name: "upper case prefix", header: sig("SHA256=" + valid), body: body, secret: secret},
		{name: "not hex", header: sig("sha256=" + strings.Repeat("zz", 32)), body: body, secret: secret},
		{name: "odd length hex", header: sig("sha256=" + valid[:63]), body: body, secret: secret},
		{name: "truncated digest", header: sig("sha256=" + valid[:62]), body: body, secret: secret},
		{name: "digest with extra bytes", header: sig("sha256=" + valid + "00"), body: body, secret: secret},
		{name: "space before digest", header: sig("sha256= " + valid), body: body, secret: secret},
		{name: "space after digest", header: sig("sha256=" + valid + " "), body: body, secret: secret},
		{name: "non-ASCII in header", header: sig("sha256=é" + valid), body: body, secret: secret},
		{name: "two lines, first valid", header: header("X-Hub-Signature-256", "sha256="+valid, "X-Hub-Signature-256", "sha256="+strings.Repeat("0", 64)), body: body, secret: secret},
		{name: "two lines, second valid", header: header("X-Hub-Signature-256", "sha256="+strings.Repeat("0", 64), "X-Hub-Signature-256", "sha256="+valid), body: body, secret: secret},
		{name: "wrong secret", header: sig("sha256=" + valid), body: body, secret: secret + "x"},
		{name: "tampered body", header: sig("sha256=" + valid), body: append([]byte(nil), append(body, ' ')...), secret: secret},
		{name: "body emptied", header: sig("sha256=" + valid), body: nil, secret: secret},
		{name: "signature of an empty body used for a body", header: sig("sha256=" + signHex(secret, nil)), body: body, secret: secret},
		{name: "signature over the body plus a newline", header: sig("sha256=" + signHex(secret, append(append([]byte(nil), body...), '\n'))), body: body, secret: secret},
		{name: "signature over the header name", header: sig("sha256=" + signHex(secret, []byte("X-Hub-Signature-256"))), body: body, secret: secret},
		{name: "empty secret", header: sig("sha256=" + valid), body: body, secret: ""},
		{name: "signature made with an empty key", header: sig("sha256=" + signHex("", body)), body: body, secret: ""},
	})
}
