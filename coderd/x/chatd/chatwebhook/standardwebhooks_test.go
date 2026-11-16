package chatwebhook_test

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatwebhook"
)

// withSpareBitsSet returns an encoded 32-byte key whose last base64
// character has its two spare bits set. The decoder reads the same key bytes
// from it, so only a strict decode tells it apart from the canonical form.
func withSpareBitsSet(t *testing.T, canonical string) string {
	t.Helper()
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	require.Equal(t, byte('='), canonical[len(canonical)-1], "a 32-byte key ends in one padding character")
	last := strings.IndexByte(alphabet, canonical[len(canonical)-2])
	require.Zero(t, last%4, "the spare bits of a canonical last character are zero")
	return canonical[:len(canonical)-2] + string(alphabet[last+1]) + "="
}

func TestStandardWebhooks(t *testing.T) {
	t.Parallel()

	// Official vector from
	// https://github.com/standard-webhooks/standard-webhooks/blob/main/libraries/go/webhook_test.go
	// (TestWebhookSign). Its timestamp is 1614265330.
	const (
		vectorSecret = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw" //nolint:gosec // Public test vector from the Standard Webhooks repository.
		vectorID     = "msg_p5jXN8AQM9LWM0D4loKWxJek"
		vectorTS     = "1614265330"
		vectorBody   = `{"test": 2432232314}`
		vectorSig    = "v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE="
	)
	vectorNow := time.Unix(1614265330, 0)

	key := []byte("0123456789abcdef0123456789abcdef")
	secret := "whsec_" + base64.StdEncoding.EncodeToString(key)
	body := []byte(`{"type":"example.event","data":{"foo":"bar"}}`)
	const id = "msg_2KWPBgLlAfxdpx2AI54pPJ85f4W"

	stamp := func(d time.Duration) string { return strconv.FormatInt(now.Add(d).Unix(), 10) }
	ts := stamp(0)
	// sign signs the string "id.timestamp.body" with key, in the v1 format.
	sign := func(key []byte, id, ts string, body []byte) string {
		return "v1," + base64.StdEncoding.EncodeToString(hmacSHA256(key, []byte(id+"."+ts+"."+string(body))))
	}
	valid := sign(key, id, ts, body)
	hdr := func(id, ts string, sigs ...string) http.Header {
		return header("webhook-id", id, "webhook-timestamp", ts, "webhook-signature", strings.Join(sigs, " "))
	}
	// badSig has the shape of a v1 entry and the wrong MAC.
	badSig := sign([]byte("another key"), id, ts, body)
	// at is a correctly signed request stamped d from now.
	at := func(d time.Duration) verifyCase {
		return verifyCase{header: hdr(id, stamp(d), sign(key, id, stamp(d), body)), body: body, secret: secret}
	}
	boundary := func(name string, d time.Duration, accepted bool) verifyCase {
		tc := at(d)
		tc.name, tc.accepted = name, accepted
		return tc
	}

	runVerifyCases(t, newVerifier(t, chatwebhook.SchemeStandardWebhooks), []verifyCase{
		{name: "official vector", header: hdr(vectorID, vectorTS, vectorSig), body: []byte(vectorBody), secret: vectorSecret, now: vectorNow, accepted: true},
		{name: "valid request", header: hdr(id, ts, valid), body: body, secret: secret, accepted: true},
		{name: "empty body signed", header: hdr(id, ts, sign(key, id, ts, nil)), body: nil, secret: secret, accepted: true},
		{name: "non-ASCII body signed", header: hdr(id, ts, sign(key, id, ts, []byte("héllo ✓"))), body: []byte("héllo ✓"), secret: secret, accepted: true},
		{name: "invalid UTF-8 body signed", header: hdr(id, ts, sign(key, id, ts, []byte{0xff, 0x00, 0x80})), body: []byte{0xff, 0x00, 0x80}, secret: secret, accepted: true},
		{name: "non-ASCII id signed", header: hdr("msg_é", ts, sign(key, "msg_é", ts, body)), body: body, secret: secret, accepted: true},
		{name: "body that starts with digits and a dot", header: hdr(id, ts, sign(key, id, ts, []byte("1.2"))), body: []byte("1.2"), secret: secret, accepted: true},

		// Timestamp window: "more than five minutes old" is rejected, and so
		// is a timestamp more than five minutes ahead.
		boundary("timestamp now", 0, true),
		boundary("timestamp five minutes old", -5*time.Minute, true),
		boundary("timestamp five minutes ahead", 5*time.Minute, true),
		boundary("timestamp one second more than five minutes old", -5*time.Minute-time.Second, false),
		boundary("timestamp one second more than five minutes ahead", 5*time.Minute+time.Second, false),
		boundary("timestamp one hour old", -time.Hour, false),
		boundary("timestamp one hour ahead", time.Hour, false),
		boundary("timestamp one year old", -365*24*time.Hour, false),
		{name: "timestamp zero", header: hdr(id, "0", sign(key, id, "0", body)), body: body, secret: secret},
		{name: "timestamp at the int64 maximum", header: hdr(id, "9223372036854775807", sign(key, id, "9223372036854775807", body)), body: body, secret: secret},
		{name: "timestamp past the int64 maximum", header: hdr(id, "9223372036854775808", sign(key, id, "9223372036854775808", body)), body: body, secret: secret},
		// The signed content holds the timestamp text as received.
		{name: "timestamp with leading zeros signed as received", header: hdr(id, "000"+ts, sign(key, id, "000"+ts, body)), body: body, secret: secret, accepted: true},
		{name: "timestamp with leading zeros signed as a number", header: hdr(id, "000"+ts, sign(key, id, ts, body)), body: body, secret: secret},
		{name: "timestamp with a sign", header: hdr(id, "+"+ts, sign(key, id, "+"+ts, body)), body: body, secret: secret},
		{name: "timestamp negative", header: hdr(id, "-1", sign(key, id, "-1", body)), body: body, secret: secret},
		{name: "timestamp with a fraction", header: hdr(id, ts+".5", sign(key, id, ts+".5", body)), body: body, secret: secret},
		{name: "timestamp in hex", header: hdr(id, "0x10", sign(key, id, "0x10", body)), body: body, secret: secret},
		{name: "timestamp with a space", header: hdr(id, ts+" ", sign(key, id, ts+" ", body)), body: body, secret: secret},
		{name: "timestamp not a number", header: hdr(id, "now", sign(key, id, "now", body)), body: body, secret: secret},
		{name: "timestamp empty", header: hdr(id, "", sign(key, id, "", body)), body: body, secret: secret},

		// Headers.
		{name: "all headers missing", header: http.Header{}, body: body, secret: secret},
		{name: "header nil", header: nil, body: body, secret: secret},
		{name: "id missing", header: header("webhook-timestamp", ts, "webhook-signature", valid), body: body, secret: secret},
		{name: "timestamp missing", header: header("webhook-id", id, "webhook-signature", valid), body: body, secret: secret},
		{name: "signature missing", header: header("webhook-id", id, "webhook-timestamp", ts), body: body, secret: secret},
		{name: "id empty", header: hdr("", ts, sign(key, "", ts, body)), body: body, secret: secret},
		{name: "id with a dot", header: hdr("msg.1", ts, sign(key, "msg.1", ts, body)), body: body, secret: secret},
		{name: "two id lines", header: header("webhook-id", id, "webhook-id", id, "webhook-timestamp", ts, "webhook-signature", valid), body: body, secret: secret},
		{name: "two timestamp lines", header: header("webhook-id", id, "webhook-timestamp", ts, "webhook-timestamp", ts, "webhook-signature", valid), body: body, secret: secret},
		{name: "signature empty", header: hdr(id, ts, ""), body: body, secret: secret},
		{name: "signature version only", header: hdr(id, ts, "v1"), body: body, secret: secret},
		{name: "signature with no MAC", header: hdr(id, ts, "v1,"), body: body, secret: secret},
		{name: "signature not base64", header: hdr(id, ts, "v1,not base64!"), body: body, secret: secret},
		{name: "signature with trailing data", header: hdr(id, ts, valid+",extra"), body: body, secret: secret},
		{name: "signature with the version in upper case", header: hdr(id, ts, "V1"+strings.TrimPrefix(valid, "v1")), body: body, secret: secret},
		{name: "signature with another version", header: hdr(id, ts, "v2"+strings.TrimPrefix(valid, "v1")), body: body, secret: secret},
		{name: "signature with the asymmetric version", header: hdr(id, ts, "v1a"+strings.TrimPrefix(valid, "v1")), body: body, secret: secret},
		{name: "signature without a version", header: hdr(id, ts, strings.TrimPrefix(valid, "v1,")), body: body, secret: secret},
		{name: "signature truncated", header: hdr(id, ts, valid[:len(valid)-4]), body: body, secret: secret},

		// Several signatures: one valid v1 entry is enough.
		{name: "bad then valid", header: hdr(id, ts, badSig, valid), body: body, secret: secret, accepted: true},
		{name: "valid then bad", header: hdr(id, ts, valid, badSig), body: body, secret: secret, accepted: true},
		{name: "spec example shape, v1a beside v1", header: hdr(id, ts, "v1a,hnO3f9T8Ytu9HwrXslvumlUpqtNVqkhqw/enGzPCXe5BdqzCInXqYXFymVJaA7AZdpXwVLPo3mNl8EM+m7TBAg==", valid), body: body, secret: secret, accepted: true},
		{name: "other versions and junk before the valid entry", header: hdr(id, ts, "v2,"+strings.TrimPrefix(valid, "v1,"), "junk", ",,,", "v1,not,base64", valid), body: body, secret: secret, accepted: true},
		{name: "extra spaces between entries", header: hdr(id, ts, badSig+"  ", valid), body: body, secret: secret, accepted: true},
		{name: "several signature lines, second valid", header: header("webhook-id", id, "webhook-timestamp", ts, "webhook-signature", badSig, "webhook-signature", valid), body: body, secret: secret, accepted: true},
		{name: "several entries, none valid", header: hdr(id, ts, badSig, badSig, "v2,"+strings.TrimPrefix(valid, "v1,")), body: body, secret: secret},

		// Signed content is "id.timestamp.body" and nothing else.
		{name: "signature for another body", header: hdr(id, ts, sign(key, id, ts, []byte("other"))), body: body, secret: secret},
		{name: "body tampered", header: hdr(id, ts, valid), body: append(append([]byte(nil), body...), ' '), secret: secret},
		{name: "body emptied", header: hdr(id, ts, valid), body: nil, secret: secret},
		{name: "signature for another id", header: hdr(id, ts, sign(key, "msg_other", ts, body)), body: body, secret: secret},
		{name: "signature for another timestamp", header: hdr(id, ts, sign(key, id, stamp(time.Second), body)), body: body, secret: secret},
		{name: "signature over timestamp and body only", header: hdr(id, ts, "v1,"+base64.StdEncoding.EncodeToString(hmacSHA256(key, []byte(ts+"."+string(body))))), body: body, secret: secret},
		{name: "signature over id and body only", header: hdr(id, ts, "v1,"+base64.StdEncoding.EncodeToString(hmacSHA256(key, []byte(id+"."+string(body))))), body: body, secret: secret},
		{name: "signature over the parts in another order", header: hdr(id, ts, "v1,"+base64.StdEncoding.EncodeToString(hmacSHA256(key, []byte(ts+"."+id+"."+string(body))))), body: body, secret: secret},
		{name: "signature over the parts without dots", header: hdr(id, ts, "v1,"+base64.StdEncoding.EncodeToString(hmacSHA256(key, []byte(id+ts+string(body))))), body: body, secret: secret},
		{name: "signature over the body only", header: hdr(id, ts, "v1,"+base64.StdEncoding.EncodeToString(hmacSHA256(key, body))), body: body, secret: secret},
		{name: "signature with the prefix inside the key", header: hdr(id, ts, sign([]byte(secret), id, ts, body)), body: body, secret: secret},

		// Secrets.
		{name: "wrong secret", header: hdr(id, ts, valid), body: body, secret: "whsec_" + base64.StdEncoding.EncodeToString([]byte("another key"))},
		{name: "secret without the prefix", header: hdr(id, ts, valid), body: body, secret: strings.TrimPrefix(secret, "whsec_")},
		{name: "secret with the prefix in upper case", header: hdr(id, ts, valid), body: body, secret: "WHSEC_" + strings.TrimPrefix(secret, "whsec_")},
		{name: "secret with the spare bits of the last base64 character set", header: hdr(id, ts, valid), body: body, secret: "whsec_" + withSpareBitsSet(t, base64.StdEncoding.EncodeToString(key))},
		{name: "secret not base64", header: hdr(id, ts, valid), body: body, secret: "whsec_not base64!"},
		{name: "secret empty", header: hdr(id, ts, valid), body: body, secret: ""},
		{name: "secret prefix only", header: hdr(id, ts, valid), body: body, secret: "whsec_"},
		{name: "signature made with an empty key", header: hdr(id, ts, sign(nil, id, ts, body)), body: body, secret: "whsec_"},
	})
}

func TestStandardWebhooksValidateSecret(t *testing.T) {
	t.Parallel()

	v := newVerifier(t, chatwebhook.SchemeStandardWebhooks)
	// Official secret, from
	// https://github.com/standard-webhooks/standard-webhooks/blob/main/libraries/go/webhook_test.go
	const official = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"

	for name, secret := range map[string]string{
		"official secret":            official,
		"one byte":                   "whsec_" + base64.StdEncoding.EncodeToString([]byte{1}),
		"64 bytes":                   "whsec_" + base64.StdEncoding.EncodeToString(make([]byte, 64)),
		"base64 with padding":        "whsec_" + base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")),
		"base64 with plus and slash": "whsec_" + base64.StdEncoding.EncodeToString([]byte{0xfb, 0xff, 0xbf, 0xfb, 0xff, 0xbf}),
	} {
		t.Run("valid "+name, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, v.ValidateSecret(secret))
		})
	}

	t.Run("invalid base64 with the spare bits of the last character set", func(t *testing.T) {
		t.Parallel()
		encoded := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
		require.NoError(t, v.ValidateSecret("whsec_"+encoded))
		require.ErrorIs(t, v.ValidateSecret("whsec_"+withSpareBitsSet(t, encoded)), chatwebhook.ErrInvalidSecret)
	})

	for name, secret := range map[string]string{
		"empty":                       "",
		"prefix only":                 "whsec_",
		"no prefix":                   strings.TrimPrefix(official, "whsec_"),
		"prefix in upper case":        "WHSEC_" + strings.TrimPrefix(official, "whsec_"),
		"another prefix":              "whsk_" + strings.TrimPrefix(official, "whsec_"),
		"prefix twice":                "whsec_" + official,
		"not base64":                  "whsec_not base64!",
		"base64 url alphabet":         "whsec_" + base64.URLEncoding.EncodeToString([]byte{0xfb, 0xff, 0xbf}),
		"base64 without padding":      "whsec_" + base64.RawStdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")),
		"base64 with a newline":       official + "\n",
		"base64 with a space":         official + " ",
		"base64 with a leading space": " " + official,
		"non-ASCII":                   "whsec_é",
	} {
		t.Run("invalid "+name, func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, v.ValidateSecret(secret), chatwebhook.ErrInvalidSecret)
		})
	}
}
