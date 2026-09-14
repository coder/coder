package dbcrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/xerrors"
)

// cipherAES256GCM is the name of the AES-256 cipher.
// This is used to identify the cipher used to encrypt a value.
// It is added to the digest to ensure that if, in the future,
// we add a new cipher type, and a key is re-used, we don't
// accidentally decrypt the wrong values.
// When adding a new cipher type, add a new constant here
// and ensure to add the cipher name to the digest of the new
// cipher type.
const (
	cipherAES256GCM = "aes256gcm"
)

type Cipher interface {
	Encrypt([]byte) ([]byte, error)
	Decrypt([]byte) ([]byte, error)
	HexDigest() string
}

// NewCiphers is a convenience function for creating multiple ciphers.
// It currently only supports AES-256-GCM.
func NewCiphers(keys ...[]byte) ([]Cipher, error) {
	var cs []Cipher
	for _, key := range keys {
		c, err := cipherAES256(key)
		if err != nil {
			return nil, err
		}
		cs = append(cs, c)
	}
	return cs, nil
}

// cipherAES256 returns a new AES-256 cipher.
func cipherAES256(key []byte) (*aes256, error) {
	if len(key) != 32 {
		return nil, xerrors.Errorf("key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	// We add the cipher name to the digest to ensure that if, in the future,
	// we add a new cipher type, and a key is re-used, we don't accidentally
	// decrypt the wrong values.
	toDigest := []byte(cipherAES256GCM)
	toDigest = append(toDigest, key...)
	digest := fmt.Sprintf("%x", sha256.Sum256(toDigest))[:7]
	return &aes256{aead: aead, digest: digest}, nil
}

type aes256 struct {
	aead cipher.AEAD
	// digest is the first 7 bytes of the hex-encoded SHA-256 digest of aead.
	digest string
}

func (a *aes256) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, a.aead.NonceSize())
	_, err := io.ReadFull(rand.Reader, nonce)
	if err != nil {
		return nil, err
	}
	dst := make([]byte, len(nonce))
	copy(dst, nonce)
	return a.aead.Seal(dst, nonce, plaintext, nil), nil
}

func (a *aes256) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < a.aead.NonceSize() {
		return nil, xerrors.Errorf("ciphertext too short")
	}
	decrypted, err := a.aead.Open(nil, ciphertext[:a.aead.NonceSize()], ciphertext[a.aead.NonceSize():], nil)
	if err != nil {
		return nil, &DecryptFailedError{Inner: err}
	}
	return decrypted, nil
}

func (a *aes256) HexDigest() string {
	return a.digest
}
