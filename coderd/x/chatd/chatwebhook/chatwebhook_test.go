package chatwebhook_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatwebhook"
)

// now is the server clock that tests pass to Verify.
var now = time.Date(2026, time.November, 11, 12, 0, 0, 0, time.UTC)

// verifyCase is one request for a verifier. accepted false means Verify must
// return an error that wraps ErrRejected.
type verifyCase struct {
	name     string
	header   http.Header
	body     []byte
	secret   string
	now      time.Time // The zero value means the package-level now.
	accepted bool
}

func newVerifier(t *testing.T, scheme chatwebhook.Scheme) chatwebhook.WebhookVerifier {
	t.Helper()
	v, err := chatwebhook.NewVerifier(scheme)
	require.NoError(t, err)
	return v
}

// header builds a header from name and value pairs. A name that repeats
// becomes several header lines.
func header(pairs ...string) http.Header {
	h := http.Header{}
	for i := 0; i < len(pairs); i += 2 {
		h.Add(pairs[i], pairs[i+1])
	}
	return h
}

func hmacSHA256(key, msg []byte) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write(msg)
	return h.Sum(nil)
}

func requireRejected(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	require.ErrorIs(t, err, chatwebhook.ErrRejected)
}

func runVerifyCases(t *testing.T, v chatwebhook.WebhookVerifier, cases []verifyCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			at := tc.now
			if at.IsZero() {
				at = now
			}
			err := v.Verify(tc.header, tc.body, tc.secret, at)
			if tc.accepted {
				require.NoError(t, err)
				return
			}
			requireRejected(t, err)
		})
	}
}

func TestNewVerifier(t *testing.T) {
	t.Parallel()

	// Every scheme has a verifier. PR B4 repeats this check for the
	// database enum.
	for _, scheme := range []chatwebhook.Scheme{
		chatwebhook.SchemeBearer,
		chatwebhook.SchemeGitHub,
		chatwebhook.SchemeStandardWebhooks,
	} {
		t.Run(string(scheme), func(t *testing.T) {
			t.Parallel()
			v, err := chatwebhook.NewVerifier(scheme)
			require.NoError(t, err)
			require.NotNil(t, v)
		})
	}

	t.Run("values match the RFC enum strings", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "bearer", string(chatwebhook.SchemeBearer))
		require.Equal(t, "github", string(chatwebhook.SchemeGitHub))
		require.Equal(t, "standard_webhooks", string(chatwebhook.SchemeStandardWebhooks))
	})

	for _, scheme := range []chatwebhook.Scheme{"", "Bearer", "hmac", "standard-webhooks"} {
		t.Run("unknown scheme "+strconv.Quote(string(scheme)), func(t *testing.T) {
			t.Parallel()
			v, err := chatwebhook.NewVerifier(scheme)
			require.Error(t, err)
			require.Nil(t, v)
		})
	}
}

func TestGenerateSecret(t *testing.T) {
	t.Parallel()

	for _, scheme := range []chatwebhook.Scheme{chatwebhook.SchemeBearer, chatwebhook.SchemeGitHub} {
		t.Run(string(scheme), func(t *testing.T) {
			t.Parallel()
			v := newVerifier(t, scheme)

			seen := map[string]struct{}{}
			for range 50 {
				secret, ok, err := v.GenerateSecret()
				require.NoError(t, err)
				require.True(t, ok)

				raw, err := base64.RawURLEncoding.DecodeString(secret)
				require.NoError(t, err, "a generated secret is base64url, which is safe in a header")
				require.GreaterOrEqual(t, len(raw), 32, "at least 32 random bytes")

				_, dup := seen[secret]
				require.False(t, dup, "generated secrets are unique")
				seen[secret] = struct{}{}
			}

			// Coder creates this secret, so a pasted one is refused.
			err := v.ValidateSecret("anything")
			require.ErrorIs(t, err, chatwebhook.ErrInvalidSecret)
		})
	}

	t.Run("generated bearer secret verifies a request", func(t *testing.T) {
		t.Parallel()
		v := newVerifier(t, chatwebhook.SchemeBearer)
		secret, _, err := v.GenerateSecret()
		require.NoError(t, err)
		require.NoError(t, v.Verify(header("Authorization", "Bearer "+secret), nil, secret, now))
	})

	t.Run("generated github secret verifies a request", func(t *testing.T) {
		t.Parallel()
		v := newVerifier(t, chatwebhook.SchemeGitHub)
		secret, _, err := v.GenerateSecret()
		require.NoError(t, err)
		body := []byte(`{"zen":"Keep it logically awesome."}`)
		sig := "sha256=" + hex.EncodeToString(hmacSHA256([]byte(secret), body))
		require.NoError(t, v.Verify(header("X-Hub-Signature-256", sig), body, secret, now))
	})

	t.Run("standard_webhooks sender creates the secret", func(t *testing.T) {
		t.Parallel()
		secret, ok, err := newVerifier(t, chatwebhook.SchemeStandardWebhooks).GenerateSecret()
		require.NoError(t, err)
		require.False(t, ok)
		require.Empty(t, secret)
	})
}

func TestVerifyRepeatedRequest(t *testing.T) {
	t.Parallel()

	// Coder does not deduplicate deliveries, so the same valid request
	// verifies every time.
	const bearerSecret = "bearer-secret"
	const githubSecret = "github-secret"
	githubBody := []byte(`{"action":"opened"}`)
	standardKey := []byte("0123456789abcdef0123456789abcdef")
	standardBody := []byte(`{"type":"example.event"}`)
	ts := strconv.FormatInt(now.Unix(), 10)

	requests := map[chatwebhook.Scheme]struct {
		header http.Header
		body   []byte
		secret string
	}{
		chatwebhook.SchemeBearer: {
			header: header("Authorization", "Bearer "+bearerSecret),
			secret: bearerSecret,
		},
		chatwebhook.SchemeGitHub: {
			header: header("X-Hub-Signature-256", "sha256="+hex.EncodeToString(hmacSHA256([]byte(githubSecret), githubBody))),
			body:   githubBody,
			secret: githubSecret,
		},
		chatwebhook.SchemeStandardWebhooks: {
			header: header(
				"webhook-id", "msg_1",
				"webhook-timestamp", ts,
				"webhook-signature", "v1,"+base64.StdEncoding.EncodeToString(hmacSHA256(standardKey, []byte("msg_1."+ts+"."+string(standardBody)))),
			),
			body:   standardBody,
			secret: "whsec_" + base64.StdEncoding.EncodeToString(standardKey),
		},
	}
	for scheme, req := range requests {
		t.Run(string(scheme), func(t *testing.T) {
			t.Parallel()
			v := newVerifier(t, scheme)
			for range 3 {
				require.NoError(t, v.Verify(req.header, req.body, req.secret, now))
			}
		})
	}
}

// errorTexts returns every text an error can show: its message, its verbose
// form, and the message of each error in its unwrap chain.
func errorTexts(err error) []string {
	texts := []string{err.Error(), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err)}
	for e := errors.Unwrap(err); e != nil; e = errors.Unwrap(e) {
		texts = append(texts, e.Error(), fmt.Sprintf("%+v", e))
	}
	return texts
}

func requireNoCanary(t *testing.T, err error, canaries ...string) {
	t.Helper()
	require.Error(t, err)
	for _, text := range errorTexts(err) {
		for _, canary := range canaries {
			require.NotContains(t, text, canary)
		}
	}
}

// badRequest is a request that Verify must reject.
type badRequest struct {
	name   string
	header http.Header
	secret string
}

// requireOpaqueRejections checks that each request is rejected and that no
// error text shows a canary.
func requireOpaqueRejections(t *testing.T, v chatwebhook.WebhookVerifier, body []byte, requests []badRequest, canaries ...string) {
	t.Helper()
	for _, req := range requests {
		t.Run(req.name, func(t *testing.T) {
			t.Parallel()
			err := v.Verify(req.header, body, req.secret, now)
			requireRejected(t, err)
			requireNoCanary(t, err, canaries...)
		})
	}
}

func TestErrorsDoNotLeakCredentials(t *testing.T) {
	t.Parallel()

	// Each canary stands for a value that must never appear in an error: the
	// stored secret, a received token or signature, an expected signature.
	const (
		secretCanary   = "CANARY-SECRET-0f9e8d7c6b5a4938" //nolint:gosec // Fixture that must never appear in an error.
		tokenCanary    = "CANARY-TOKEN-1a2b3c4d5e6f7089"  //nolint:gosec // Fixture that must never appear in an error.
		receivedCanary = "CANARY-RECEIVED-77665544"
	)

	t.Run("bearer", func(t *testing.T) {
		t.Parallel()
		v := newVerifier(t, chatwebhook.SchemeBearer)
		requireOpaqueRejections(t, v, nil, []badRequest{
			{"wrong token", header("Authorization", "Bearer "+tokenCanary), secretCanary},
			{"wrong scheme", header("Authorization", "Basic "+tokenCanary), secretCanary},
			{"two lines", header("Authorization", "Bearer "+tokenCanary, "Authorization", "Bearer "+secretCanary), secretCanary},
			{"empty secret", header("Authorization", "Bearer "+tokenCanary), ""},
		}, secretCanary, tokenCanary)
		requireNoCanary(t, v.ValidateSecret(secretCanary), secretCanary)
	})

	t.Run("github", func(t *testing.T) {
		t.Parallel()
		v := newVerifier(t, chatwebhook.SchemeGitHub)
		body := []byte(`{"a":1}`)
		expected := hex.EncodeToString(hmacSHA256([]byte(secretCanary), body))
		wrong := hex.EncodeToString(hmacSHA256([]byte("other"), body))
		sig := func(v string) http.Header { return header("X-Hub-Signature-256", v) }
		requireOpaqueRejections(t, v, body, []badRequest{
			{"wrong signature", sig("sha256=" + wrong), secretCanary},
			{"not hex", sig("sha256=" + receivedCanary), secretCanary},
			{"no prefix", sig(wrong), secretCanary},
			{"legacy sha1 prefix", sig("sha1=" + expected), secretCanary},
			{"two lines", header("X-Hub-Signature-256", "sha256="+wrong, "X-Hub-Signature-256", "sha256="+expected), secretCanary},
			{"truncated", sig("sha256=" + wrong[:20]), secretCanary},
			{"empty secret", sig("sha256=" + expected), ""},
		}, secretCanary, expected, "sha256="+expected, wrong, receivedCanary)
	})

	t.Run("standard_webhooks", func(t *testing.T) {
		t.Parallel()
		v := newVerifier(t, chatwebhook.SchemeStandardWebhooks)
		key := []byte(secretCanary)
		keyB64 := base64.StdEncoding.EncodeToString(key)
		secret := "whsec_" + keyB64
		body := []byte(`{"a":1}`)
		ts := strconv.FormatInt(now.Unix(), 10)
		old := strconv.FormatInt(now.Add(-time.Hour).Unix(), 10)
		signature := func(key []byte, ts string) string {
			return base64.StdEncoding.EncodeToString(hmacSHA256(key, []byte("msg_1."+ts+"."+string(body))))
		}
		expected, wrong, oldSig := signature(key, ts), signature([]byte("other"), ts), signature(key, old)
		hdr := func(ts string, sigs ...string) http.Header {
			return header("webhook-id", "msg_1", "webhook-timestamp", ts, "webhook-signature", strings.Join(sigs, " "))
		}
		requireOpaqueRejections(t, v, body, []badRequest{
			{"wrong signature", hdr(ts, "v1,"+wrong), secret},
			{"timestamp too old", hdr(old, "v1,"+oldSig), secret},
			{"not base64", hdr(ts, "v1,"+receivedCanary), secret},
			{"wrong version", hdr(ts, "v2,"+expected), secret},
			{"trailing data", hdr(ts, "v1,"+expected+","+receivedCanary), secret},
			{"missing id", header("webhook-timestamp", ts, "webhook-signature", "v1,"+expected), secret},
			{"secret without prefix", hdr(ts, "v1,"+expected), keyB64},
			{"secret not base64", hdr(ts, "v1,"+expected), secret + "!"},
			{"empty secret", hdr(ts, "v1,"+expected), ""},
			{"prefix only secret", hdr(ts, "v1,"+expected), "whsec_"},
		}, secretCanary, keyB64, secret, expected, "v1,"+expected, wrong, oldSig, receivedCanary)

		for _, bad := range []string{secret + "!", keyB64, "", "whsec_"} {
			err := v.ValidateSecret(bad)
			require.ErrorIs(t, err, chatwebhook.ErrInvalidSecret)
			requireNoCanary(t, err, secretCanary, keyB64)
		}
	})
}
