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

type Cipher interface {
	Encrypt([]byte) ([]byte, error)
	Decrypt([]byte) ([]byte, error)
	HexDigest() string
}

// CipherAES256 returns a new AES-256 cipher.
func CipherAES256(key []byte) (*AES256, error) {
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
	digest := fmt.Sprintf("%x", sha256.Sum256(key))[:7]
	return &AES256{aead: aead, digest: digest}, nil
}

type AES256 struct {
	aead cipher.AEAD
	// digest is the first 7 bytes of the hex-encoded SHA-256 digest of aead.
	digest string
}

func (a *AES256) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, a.aead.NonceSize())
	_, err := io.ReadFull(rand.Reader, nonce)
	if err != nil {
		return nil, err
	}
	return a.aead.Seal(nonce, nonce, plaintext, nil), nil
}

func (a *AES256) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < a.aead.NonceSize() {
		return nil, xerrors.Errorf("ciphertext too short")
	}
	decrypted, err := a.aead.Open(nil, ciphertext[:a.aead.NonceSize()], ciphertext[a.aead.NonceSize():], nil)
	if err != nil {
		return nil, &DecryptFailedError{Inner: err}
	}
	return decrypted, nil
}

func (a *AES256) HexDigest() string {
	return a.digest
}

type (
	CipherDigest string
	Ciphers      struct {
		primary string
		m       map[string]Cipher
	}
)

// NewCiphers returns a new Ciphers instance with the given ciphers.
// The first cipher in the list is the primary cipher. Any ciphers after the
// first are considered secondary ciphers and are only used for decryption.
func NewCiphers(cs ...Cipher) *Ciphers {
	var primary string
	m := make(map[string]Cipher)
	for idx, c := range cs {
		if _, ok := c.(*Ciphers); ok {
			panic("developer error: do not nest Ciphers")
		}
		m[c.HexDigest()] = c
		if idx == 0 {
			primary = c.HexDigest()
		}
	}
	return &Ciphers{primary: primary, m: m}
}

// Encrypt encrypts the given plaintext using the primary cipher and returns the
// ciphertext. The ciphertext is prefixed with the primary cipher's digest.
func (cs Ciphers) Encrypt(plaintext []byte) ([]byte, error) {
	c, ok := cs.m[cs.primary]
	if !ok {
		return nil, xerrors.Errorf("no ciphers configured")
	}
	prefix := []byte(c.HexDigest() + "-")
	crypted, err := c.Encrypt(plaintext)
	if err != nil {
		return nil, err
	}
	return append(prefix, crypted...), nil
}

// Decrypt decrypts the given ciphertext using the cipher indicated by the
// ciphertext's prefix. The prefix is the first 7 bytes of the hex-encoded
// SHA-256 digest of the cipher's key. Decryption will fail if the prefix
// does not match any of the configured ciphers.
func (cs Ciphers) Decrypt(ciphertext []byte) ([]byte, error) {
	requiredPrefix := string(ciphertext[:7])
	c, ok := cs.m[requiredPrefix]
	if !ok {
		return nil, xerrors.Errorf("missing required decryption cipher %s", requiredPrefix)
	}
	return c.Decrypt(ciphertext[8:])
}

// HexDigest returns the digest of the primary cipher.
func (cs Ciphers) HexDigest() string {
	return cs.primary
}
