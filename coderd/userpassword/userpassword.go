package userpassword

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/xerrors"
)

const (
	// This is the length of our output hash.
	// bcrypt has a hash size of 59, so we rounded up to a power of 8.
	hashLength = 64
	// The scheme to include in our hashed password.
	hashScheme = "pbkdf2-sha256"
)

// Compare checks the equality of passwords from a hashed pbkdf2 string.
// This uses pbkdf2 to ensure FIPS 140-2 compliance. See:
// https://csrc.nist.gov/csrc/media/projects/cryptographic-module-validation-program/documents/security-policies/140sp2261.pdf
func Compare(hashed string, password string) (bool, error) {
	if len(hashed) < hashLength {
		return false, xerrors.Errorf("hash too short: %d", len(hashed))
	}
	parts := strings.SplitN(hashed, "$", 5)
	if len(parts) != 5 {
		return false, xerrors.Errorf("hash has too many parts: %d", len(parts))
	}
	if len(parts[0]) != 0 {
		return false, xerrors.Errorf("hash prefix is invalid")
	}
	if parts[1] != hashScheme {
		return false, xerrors.Errorf("hash isn't %q scheme: %q", hashScheme, parts[1])
	}
	iter, err := strconv.Atoi(parts[2])
	if err != nil {
		return false, xerrors.Errorf("parse iter from hash: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false, xerrors.Errorf("decode salt: %w", err)
	}

	if subtle.ConstantTimeCompare([]byte(hashWithSaltAndIter(password, salt, iter)), []byte(hashed)) != 1 {
		return false, nil
	}
	return true, nil
}

// Hash generates a hash using pbkdf2.
// See the Compare() comment for rationale.
func Hash(password string) (string, error) {
	// bcrypt uses a salt size of 16 bytes.
	salt := make([]byte, 16)
	_, err := rand.Read(salt)
	if err != nil {
		return "", xerrors.Errorf("read random bytes for salt: %w", err)
	}
	// The default hash iteration is 1024 for speed.
	// As this is increased, the password is hashed more.
	return hashWithSaltAndIter(password, salt, 1024), nil
}

// Produces a string representation of the hash.
func hashWithSaltAndIter(password string, salt []byte, iter int) string {
	hash := pbkdf2.Key([]byte(password), salt, iter, hashLength, sha256.New)
	hash = []byte(base64.RawStdEncoding.EncodeToString(hash))
	salt = []byte(base64.RawStdEncoding.EncodeToString(salt))
	// This format is similar to bcrypt. See:
	// https://en.wikipedia.org/wiki/Bcrypt#Description
	return fmt.Sprintf("$%s$%d$%s$%s", hashScheme, iter, salt, hash)
}
