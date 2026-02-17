package coderd

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"math/big"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestValidateECP256PublicKey(t *testing.T) {
	t.Parallel()

	t.Run("ValidKey", func(t *testing.T) {
		t.Parallel()
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		raw := elliptic.Marshal(elliptic.P256(), key.PublicKey.X, key.PublicKey.Y)
		require.NoError(t, validateECP256PublicKey(raw))
	})

	t.Run("WrongLength", func(t *testing.T) {
		t.Parallel()
		err := validateECP256PublicKey([]byte("too short"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected 65 bytes")
	})

	t.Run("WrongPrefix", func(t *testing.T) {
		t.Parallel()
		raw := make([]byte, 65)
		raw[0] = 0x02 // compressed prefix, not uncompressed
		err := validateECP256PublicKey(raw)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "0x04")
	})

	t.Run("NotOnCurve", func(t *testing.T) {
		t.Parallel()
		raw := make([]byte, 65)
		raw[0] = 0x04
		// X and Y are zero, which is not on P-256.
		err := validateECP256PublicKey(raw)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not on the P-256 curve")
	})
}

func TestParseRawECP256PublicKey(t *testing.T) {
	t.Parallel()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	raw := elliptic.Marshal(elliptic.P256(), key.PublicKey.X, key.PublicKey.Y)

	parsed, err := parseRawECP256PublicKey(raw)
	require.NoError(t, err)
	assert.Equal(t, key.PublicKey.X, parsed.X)
	assert.Equal(t, key.PublicKey.Y, parsed.Y)
}

// generateTestProof creates a valid ConnectProof using the given
// private key and timestamp.
func generateTestProof(t *testing.T, key *ecdsa.PrivateKey, timestamp int64) string {
	t.Helper()
	tsStr := strconv.FormatInt(timestamp, 10)
	digest := sha256.Sum256([]byte(tsStr))
	sig, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	require.NoError(t, err)
	proof := codersdk.ConnectProof{
		Timestamp: timestamp,
		Signature: base64.StdEncoding.EncodeToString(sig),
	}
	encoded, err := codersdk.EncodeConnectProof(proof)
	require.NoError(t, err)
	return encoded
}

func TestVerifyConnectProof(t *testing.T) {
	t.Parallel()

	// Generate a test keypair.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	rawPub := elliptic.Marshal(elliptic.P256(), key.PublicKey.X, key.PublicKey.Y)

	t.Run("ValidProof", func(t *testing.T) {
		t.Parallel()
		proof := generateTestProof(t, key, time.Now().Unix())
		err := verifyConnectProof(proof, rawPub)
		require.NoError(t, err)
	})

	t.Run("EmptyHeader", func(t *testing.T) {
		t.Parallel()
		err := verifyConnectProof("", rawPub)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing")
	})

	t.Run("MalformedJSON", func(t *testing.T) {
		t.Parallel()
		err := verifyConnectProof("not-json", rawPub)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "malformed")
	})

	t.Run("ExpiredTimestamp", func(t *testing.T) {
		t.Parallel()
		// 5 minutes ago — well outside the 30s tolerance.
		proof := generateTestProof(t, key, time.Now().Add(-5*time.Minute).Unix())
		err := verifyConnectProof(proof, rawPub)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "out of range")
	})

	t.Run("FutureTimestamp", func(t *testing.T) {
		t.Parallel()
		proof := generateTestProof(t, key, time.Now().Add(5*time.Minute).Unix())
		err := verifyConnectProof(proof, rawPub)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "out of range")
	})

	t.Run("WrongKey", func(t *testing.T) {
		t.Parallel()
		// Sign with a different key.
		otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		proof := generateTestProof(t, otherKey, time.Now().Unix())
		err = verifyConnectProof(proof, rawPub)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "verification failed")
	})

	t.Run("InvalidSignatureEncoding", func(t *testing.T) {
		t.Parallel()
		proof := codersdk.ConnectProof{
			Timestamp: time.Now().Unix(),
			Signature: "not-valid-base64!!!",
		}
		encoded, err := codersdk.EncodeConnectProof(proof)
		require.NoError(t, err)
		err = verifyConnectProof(encoded, rawPub)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid signature encoding")
	})

	t.Run("TamperedSignature", func(t *testing.T) {
		t.Parallel()
		// Valid signature but we flip a byte.
		tsStr := strconv.FormatInt(time.Now().Unix(), 10)
		digest := sha256.Sum256([]byte(tsStr))
		sig, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
		require.NoError(t, err)
		sig[len(sig)-1] ^= 0xFF // flip last byte
		proof := codersdk.ConnectProof{
			Timestamp: time.Now().Unix(),
			Signature: base64.StdEncoding.EncodeToString(sig),
		}
		encoded, err := codersdk.EncodeConnectProof(proof)
		require.NoError(t, err)
		err = verifyConnectProof(encoded, rawPub)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "verification failed")
	})

	t.Run("TimestampWithinTolerance", func(t *testing.T) {
		t.Parallel()
		// 20 seconds ago — within 30s tolerance.
		proof := generateTestProof(t, key, time.Now().Add(-20*time.Second).Unix())
		err := verifyConnectProof(proof, rawPub)
		require.NoError(t, err)
	})
}

func TestConnectAuthRequired(t *testing.T) {
	t.Parallel()

	t.Run("EmptyList", func(t *testing.T) {
		t.Parallel()
		assert.False(t, connectAuthRequired(nil, "ssh"))
		assert.False(t, connectAuthRequired([]string{}, "ssh"))
	})

	t.Run("MatchFound", func(t *testing.T) {
		t.Parallel()
		endpoints := []string{"ssh", "port-forward"}
		assert.True(t, connectAuthRequired(endpoints, "ssh"))
		assert.True(t, connectAuthRequired(endpoints, "port-forward"))
	})

	t.Run("NoMatch", func(t *testing.T) {
		t.Parallel()
		endpoints := []string{"ssh"}
		assert.False(t, connectAuthRequired(endpoints, "port-forward"))
		assert.False(t, connectAuthRequired(endpoints, "apps"))
	})
}

func TestParseRawECP256PublicKey_RoundTrip(t *testing.T) {
	t.Parallel()

	// Generate key, marshal, parse, verify signature.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	raw := elliptic.Marshal(elliptic.P256(), key.PublicKey.X, key.PublicKey.Y)
	parsed, err := parseRawECP256PublicKey(raw)
	require.NoError(t, err)

	// Sign with private key, verify with parsed public key.
	msg := sha256.Sum256([]byte("test message"))
	r, s, err := ecdsa.Sign(rand.Reader, key, msg[:])
	require.NoError(t, err)
	assert.True(t, ecdsa.Verify(parsed, msg[:], r, s))

	// Verify wrong message fails.
	wrongMsg := sha256.Sum256([]byte("wrong message"))
	assert.False(t, ecdsa.Verify(parsed, wrongMsg[:], r, s))
}

func TestValidateECP256PublicKey_P384Rejected(t *testing.T) {
	t.Parallel()

	// P-384 key is 97 bytes (not 65), should be rejected.
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	require.NoError(t, err)
	raw := elliptic.Marshal(elliptic.P384(), key.PublicKey.X, key.PublicKey.Y)
	err = validateECP256PublicKey(raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected 65 bytes")
}

func TestValidateECP256PublicKey_PointNotOnCurve(t *testing.T) {
	t.Parallel()

	// Valid-looking key (correct prefix and length) but point
	// is not on curve.
	raw := make([]byte, 65)
	raw[0] = 0x04
	// Set X to 1, Y to 1 — not on P-256.
	raw[32] = 1
	raw[64] = 1
	err := validateECP256PublicKey(raw)
	require.Error(t, err)

	// Verify that big.Int correctly loaded.
	x := new(big.Int).SetBytes(raw[1:33])
	y := new(big.Int).SetBytes(raw[33:65])
	assert.False(t, elliptic.P256().IsOnCurve(x, y))
}
