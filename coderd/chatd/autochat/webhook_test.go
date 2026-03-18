package autochat_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/chatd/autochat"
)

func TestVerifyWebhookSignature(t *testing.T) {
	t.Parallel()

	const secret = "test-secret-key"

	sign := func(body []byte) string {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		return hex.EncodeToString(mac.Sum(nil))
	}

	t.Run("ValidSignature", func(t *testing.T) {
		t.Parallel()
		body := []byte(`{"action":"opened"}`)
		header := http.Header{}
		header.Set("X-Hub-Signature-256", sign(body))

		err := autochat.VerifyWebhookSignature(body, header, secret)
		require.NoError(t, err)
	})

	t.Run("MissingHeader", func(t *testing.T) {
		t.Parallel()
		body := []byte(`{"action":"opened"}`)
		header := http.Header{}

		err := autochat.VerifyWebhookSignature(body, header, secret)
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing webhook signature header")
	})

	t.Run("WrongSignature", func(t *testing.T) {
		t.Parallel()
		body := []byte(`{"action":"opened"}`)
		header := http.Header{}
		header.Set("X-Hub-Signature-256", "deadbeef")

		err := autochat.VerifyWebhookSignature(body, header, secret)
		require.Error(t, err)
		require.Contains(t, err.Error(), "signature mismatch")
	})

	t.Run("SHA256Prefix", func(t *testing.T) {
		t.Parallel()
		body := []byte(`{"event":"push"}`)
		header := http.Header{}
		header.Set("X-Hub-Signature-256", "sha256="+sign(body))

		err := autochat.VerifyWebhookSignature(body, header, secret)
		require.NoError(t, err)
	})

	t.Run("CoderSignatureFallback", func(t *testing.T) {
		t.Parallel()
		body := []byte(`{"trigger":"cron"}`)
		header := http.Header{}
		header.Set("X-Coder-Signature", sign(body))

		err := autochat.VerifyWebhookSignature(body, header, secret)
		require.NoError(t, err)
	})
}

func TestGenerateWebhookSecret(t *testing.T) {
	t.Parallel()

	secret, err := autochat.GenerateWebhookSecret()
	require.NoError(t, err)
	// 32 random bytes = 64 hex characters.
	require.Len(t, secret, 64)

	// Verify it's valid hex.
	_, err = hex.DecodeString(secret)
	require.NoError(t, err)

	// Two calls should produce different secrets.
	secret2, err := autochat.GenerateWebhookSecret()
	require.NoError(t, err)
	require.NotEqual(t, secret, secret2)
}

func TestMaskSecret(t *testing.T) {
	t.Parallel()

	t.Run("LongSecret", func(t *testing.T) {
		t.Parallel()
		masked := autochat.MaskSecret("abcdef1234567890")
		require.Equal(t, "abcdef12****", masked)
	})

	t.Run("ShortSecret", func(t *testing.T) {
		t.Parallel()
		masked := autochat.MaskSecret("short")
		require.Equal(t, "****", masked)
	})

	t.Run("ExactlyEightChars", func(t *testing.T) {
		t.Parallel()
		masked := autochat.MaskSecret("12345678")
		require.Equal(t, "****", masked)
	})

	t.Run("NineChars", func(t *testing.T) {
		t.Parallel()
		masked := autochat.MaskSecret("123456789")
		require.Equal(t, "12345678****", masked)
	})

	t.Run("EmptyString", func(t *testing.T) {
		t.Parallel()
		masked := autochat.MaskSecret("")
		require.Equal(t, "****", masked)
	})
}
