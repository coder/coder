package main

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
	"github.com/stretchr/testify/require"
)

const testUserID = "Coder Release Signing Key <security@coder.com>"

// opaqueSigner hides the concrete *rsa.PrivateKey type, like a KMS-backed
// signer does, so the tests exercise the crypto.Signer path.
type opaqueSigner struct {
	key *rsa.PrivateKey
}

func (s opaqueSigner) Public() crypto.PublicKey { return &s.key.PublicKey }

func (s opaqueSigner) Sign(r io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	return s.key.Sign(r, digest, opts)
}

func newTestSigner(t *testing.T) crypto.Signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return opaqueSigner{key: key}
}

func TestPublicKeyIsDeterministic(t *testing.T) {
	t.Parallel()

	signer := newTestSigner(t)
	created := time.Unix(1794700000, 0)

	export := func() []byte {
		e, err := newEntity(signer, created)
		require.NoError(t, err)
		var buf bytes.Buffer
		require.NoError(t, writePublicKey(&buf, e))
		return buf.Bytes()
	}
	first := export()
	require.Equal(t, first, export())

	keyring, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(first))
	require.NoError(t, err)
	require.Len(t, keyring, 1)
	key := keyring[0]
	require.Equal(t, created.Unix(), key.PrimaryKey.CreationTime.Unix())
	require.Empty(t, key.Subkeys)
	require.Contains(t, key.Identities, testUserID)

	selfSig := key.Identities[testUserID].SelfSignature
	require.NotNil(t, selfSig.KeyLifetimeSecs)
	require.Equal(t, uint32(keyLifetime/time.Second), *selfSig.KeyLifetimeSecs)
	require.True(t, selfSig.FlagSign)
	require.True(t, selfSig.FlagCertify)
}

func TestSignWithPrependedSignature(t *testing.T) {
	t.Parallel()

	message := []byte("coder_v2.0.0_checksums.txt contents\n")

	// The old key is an ordinary software key, like the one gpg signs with.
	oldKey, err := openpgp.NewEntity("Old Key", "", "old@example.com", &packet.Config{RSABits: 2048, Algorithm: packet.PubKeyAlgoRSA})
	require.NoError(t, err)
	var oldSig bytes.Buffer
	require.NoError(t, openpgp.DetachSign(&oldSig, oldKey, bytes.NewReader(message), nil))

	newKey, err := newEntity(newTestSigner(t), time.Unix(1794700000, 0))
	require.NoError(t, err)

	var out bytes.Buffer
	require.NoError(t, sign(&out, newKey, bytes.NewReader(message), oldSig.Bytes(), time.Now()))

	// The output holds both signatures, with the old key's first. The VS Code
	// extension verifies the first signature against the old key.
	block, err := armor.Decode(bytes.NewReader(out.Bytes()))
	require.NoError(t, err)
	require.Equal(t, openpgp.SignatureType, block.Type)
	reader := packet.NewReader(block.Body)
	var issuers []uint64
	for {
		p, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		sig, ok := p.(*packet.Signature)
		require.True(t, ok, "unexpected packet %T", p)
		require.NotNil(t, sig.IssuerKeyId)
		issuers = append(issuers, *sig.IssuerKeyId)
	}
	require.Equal(t, []uint64{oldKey.PrimaryKey.KeyId, newKey.PrimaryKey.KeyId}, issuers)

	// Each key verifies its own signature in the combined output.
	for _, key := range []*openpgp.Entity{oldKey, newKey} {
		signer, err := openpgp.CheckArmoredDetachedSignature(openpgp.EntityList{key}, bytes.NewReader(message), bytes.NewReader(out.Bytes()), nil)
		require.NoError(t, err)
		require.Equal(t, key.PrimaryKey.KeyId, signer.PrimaryKey.KeyId)
	}

	// A modified message fails verification.
	_, err = openpgp.CheckArmoredDetachedSignature(openpgp.EntityList{newKey}, bytes.NewReader([]byte("tampered")), bytes.NewReader(out.Bytes()), nil)
	require.Error(t, err)
}

func TestSignRejectsArmoredPrepend(t *testing.T) {
	t.Parallel()

	newKey, err := newEntity(newTestSigner(t), time.Unix(1794700000, 0))
	require.NoError(t, err)

	var armored bytes.Buffer
	aw, err := armor.Encode(&armored, openpgp.SignatureType, nil)
	require.NoError(t, err)
	require.NoError(t, openpgp.DetachSign(aw, newKey, bytes.NewReader([]byte("x")), nil))
	require.NoError(t, aw.Close())

	var out bytes.Buffer
	err = sign(&out, newKey, bytes.NewReader([]byte("x")), armored.Bytes(), time.Now())
	require.ErrorContains(t, err, "prepend signature")
}
