package protocolone

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
)

// KDF derives a key from the shared secret using SHA-256.
func KDF(z []byte) []byte {
	hasher := sha256.New()
	hasher.Write(z)
	return hasher.Sum(nil)
}

// Encrypt encrypts the plaintext using AES-256-CBC with a zero IV.
// The key is the shared secret.
func Encrypt(sharedSecret []byte, demPlaintext []byte) ([]byte, error) {
	if len(sharedSecret) != 32 {
		return nil, errors.New("invalid shared secret length")
	}
	if len(demPlaintext)%16 != 0 {
		return nil, errors.New("invalid plaintext length")
	}

	// Encrypt PIN using AES-CBC using null IV
	block, err := aes.NewCipher(sharedSecret)
	if err != nil {
		return nil, fmt.Errorf("cannot create new AES cipher: %w", err)
	}

	iv := make([]byte, block.BlockSize())
	ciphertext := make([]byte, len(demPlaintext))

	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, demPlaintext)

	return ciphertext, nil
}

// Decrypt decrypts the ciphertext using AES-256-CBC with a zero IV.
func Decrypt(sharedSecret []byte, demCiphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(sharedSecret)
	if err != nil {
		return nil, fmt.Errorf("cannot create new AES cipher: %w", err)
	}
	if len(demCiphertext)%block.BlockSize() != 0 {
		return nil, errors.New("invalid ciphertext length")
	}

	iv := make([]byte, block.BlockSize())
	plaintext := make([]byte, len(demCiphertext))

	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(plaintext, demCiphertext)

	return plaintext, nil
}

// Authenticate calculates the HMAC-SHA-256 of the message using the shared secret.
// It returns the first 16 bytes of the HMAC.
func Authenticate(sharedSecret []byte, message []byte) []byte {
	hasher := hmac.New(sha256.New, sharedSecret)
	hasher.Write(message)
	return hasher.Sum(nil)[:16]
}
