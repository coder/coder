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

		// Timestamp window: "more than five minutes old" is rejected, and so
		// is a timestamp more than five minutes ahead.
		boundary("timestamp five minutes old", -5*time.Minute, true),
		boundary("timestamp five minutes ahead", 5*time.Minute, true),
		boundary("timestamp one second more than five minutes old", -5*time.Minute-time.Second, false),
		boundary("timestamp one second more than five minutes ahead", 5*time.Minute+time.Second, false),
		{name: "timestamp not a number", header: hdr(id, "now", sign(key, id, "now", body)), body: body, secret: secret},

		// Headers.
		{name: "id missing", header: header("webhook-timestamp", ts, "webhook-signature", valid), body: body, secret: secret},
		{name: "timestamp missing", header: header("webhook-id", id, "webhook-signature", valid), body: body, secret: secret},
		{name: "signature missing", header: header("webhook-id", id, "webhook-timestamp", ts), body: body, secret: secret},
		{name: "id empty", header: hdr("", ts, sign(key, "", ts, body)), body: body, secret: secret},
		{name: "id with a dot", header: hdr("msg.1", ts, sign(key, "msg.1", ts, body)), body: body, secret: secret},
		{name: "two id lines", header: header("webhook-id", id, "webhook-id", id, "webhook-timestamp", ts, "webhook-signature", valid), body: body, secret: secret},
		{name: "two timestamp lines", header: header("webhook-id", id, "webhook-timestamp", ts, "webhook-timestamp", ts, "webhook-signature", valid), body: body, secret: secret},
		{name: "signature with the asymmetric version", header: hdr(id, ts, "v1a"+strings.TrimPrefix(valid, "v1")), body: body, secret: secret},
		{name: "signature truncated", header: hdr(id, ts, valid[:len(valid)-4]), body: body, secret: secret},

		// Several signatures: one valid v1 entry is enough.
		{name: "bad then valid", header: hdr(id, ts, badSig, valid), body: body, secret: secret, accepted: true},
		{name: "valid then bad", header: hdr(id, ts, valid, badSig), body: body, secret: secret, accepted: true},
		{name: "other versions and junk before the valid entry", header: hdr(id, ts, "v2,"+strings.TrimPrefix(valid, "v1,"), "junk", ",,,", "v1,not,base64", valid), body: body, secret: secret, accepted: true},
		{name: "several signature lines, second valid", header: header("webhook-id", id, "webhook-timestamp", ts, "webhook-signature", badSig, "webhook-signature", valid), body: body, secret: secret, accepted: true},
		{name: "several entries, none valid", header: hdr(id, ts, badSig, badSig, "v2,"+strings.TrimPrefix(valid, "v1,")), body: body, secret: secret},

		{name: "body tampered", header: hdr(id, ts, valid), body: append(append([]byte(nil), body...), ' '), secret: secret},

		// Secrets.
		{name: "wrong secret", header: hdr(id, ts, valid), body: body, secret: "whsec_" + base64.StdEncoding.EncodeToString([]byte("another key"))},
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
		"official secret": official,
		"one byte":        "whsec_" + base64.StdEncoding.EncodeToString([]byte{1}),
	} {
		t.Run("valid "+name, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, v.ValidateSecret(secret))
		})
	}

	for name, secret := range map[string]string{
		"prefix only":            "whsec_",
		"no prefix":              strings.TrimPrefix(official, "whsec_"),
		"base64 url alphabet":    "whsec_" + base64.URLEncoding.EncodeToString([]byte{0xfb, 0xff, 0xbf}),
		"base64 without padding": "whsec_" + base64.RawStdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")),
		"base64 with a newline":  official + "\n",
	} {
		t.Run("invalid "+name, func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, v.ValidateSecret(secret), chatwebhook.ErrInvalidSecret)
		})
	}
}
