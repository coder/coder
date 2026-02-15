package codersdk_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestEncodeDecodeConnectProof(t *testing.T) {
	t.Parallel()

	proof := codersdk.ConnectProof{
		Timestamp: 1700000000,
		Signature: "dGVzdC1zaWduYXR1cmU=",
	}

	encoded, err := codersdk.EncodeConnectProof(proof)
	require.NoError(t, err)
	assert.NotEmpty(t, encoded)

	decoded, err := codersdk.DecodeConnectProof(encoded)
	require.NoError(t, err)
	assert.Equal(t, proof.Timestamp, decoded.Timestamp)
	assert.Equal(t, proof.Signature, decoded.Signature)
}

func TestDecodeConnectProof_Invalid(t *testing.T) {
	t.Parallel()

	_, err := codersdk.DecodeConnectProof("not-json")
	require.Error(t, err)
}

func TestDecodeConnectProof_Empty(t *testing.T) {
	t.Parallel()

	_, err := codersdk.DecodeConnectProof("")
	require.Error(t, err)
}
