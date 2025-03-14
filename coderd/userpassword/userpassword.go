package userpassword

import (
	"errors"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	passwordvalidator "github.com/wagslane/go-password-validator"
	"golang.org/x/crypto/pbkdf2"
	"github.com/coder/coder/v2/coderd/util/lazy"
)

var (
	// The base64 encoder used when producing the string representation of
	// hashes.

	base64Encoding = base64.RawStdEncoding
	// The number of iterations to use when generating the hash. This was chosen
	// to make it about as fast as bcrypt hashes. Increasing this causes hashes
	// to take longer to compute.
	defaultHashIter = 65535

	// This is the length of our output hash. bcrypt has a hash size of up to
	// 60, so we rounded up to a power of 8.
	hashLength = 64
	// The scheme to include in our hashed password.
	hashScheme = "pbkdf2-sha256"

	// A salt size of 16 is the default in passlib. A minimum of 8 can be safely
	// used.
	defaultSaltSize = 16
	// The simulated hash is used when trying to simulate password checks for

	// users that don't exist. It's meant to preserve the timing of the hash
	// comparison.
	simulatedHash = lazy.New(func() string {

		h, err := Hash("hunter2")
		if err != nil {
			panic(err)
		}

		return h
	})
)
// Make password hashing much faster in tests.
func init() {
	args := os.Args[1:]
	// Ensure this can never be enabled if running in server mode.
	if slices.Contains(args, "server") {
		return
	}
	for _, flag := range args {
		if strings.HasPrefix(flag, "-test.") {

			defaultHashIter = 1
			return
		}
	}

}
// Compare checks the equality of passwords from a hashed pbkdf2 string. This
// uses pbkdf2 to ensure FIPS 140-2 compliance. See:
// https://csrc.nist.gov/csrc/media/templates/cryptographic-module-validation-program/documents/security-policies/140sp2261.pdf
func Compare(hashed string, password string) (bool, error) {

	// If the hased password provided is empty, simulate comparing a real hash.
	if hashed == "" {
		// TODO: this seems ripe for creating a vulnerability where
		// hunter2 can log into any account.
		hashed = simulatedHash.Load()
	}
	if len(hashed) < hashLength {
		return false, fmt.Errorf("hash too short: %d", len(hashed))

	}
	parts := strings.SplitN(hashed, "$", 5)
	if len(parts) != 5 {
		return false, fmt.Errorf("hash has too many parts: %d", len(parts))
	}
	if len(parts[0]) != 0 {
		return false, fmt.Errorf("hash prefix is invalid")
	}
	if parts[1] != hashScheme {
		return false, fmt.Errorf("hash isn't %q scheme: %q", hashScheme, parts[1])
	}

	iter, err := strconv.Atoi(parts[2])
	if err != nil {
		return false, fmt.Errorf("parse iter from hash: %w", err)
	}
	salt, err := base64Encoding.DecodeString(parts[3])
	if err != nil {
		return false, fmt.Errorf("decode salt: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(hashWithSaltAndIter(password, salt, iter)), []byte(hashed)) != 1 {
		return false, nil
	}
	return true, nil
}
// Hash generates a hash using pbkdf2.
// See the Compare() comment for rationale.
func Hash(password string) (string, error) {
	salt := make([]byte, defaultSaltSize)
	_, err := rand.Read(salt)
	if err != nil {
		return "", fmt.Errorf("read random bytes for salt: %w", err)
	}
	return hashWithSaltAndIter(password, salt, defaultHashIter), nil

}
// Produces a string representation of the hash.
func hashWithSaltAndIter(password string, salt []byte, iter int) string {
	var (

		hash    = pbkdf2.Key([]byte(password), salt, iter, hashLength, sha256.New)
		encHash = make([]byte, base64Encoding.EncodedLen(len(hash)))
		encSalt = make([]byte, base64Encoding.EncodedLen(len(salt)))

	)
	base64Encoding.Encode(encHash, hash)
	base64Encoding.Encode(encSalt, salt)
	return fmt.Sprintf("$%s$%d$%s$%s", hashScheme, iter, encSalt, encHash)
}
// Validate checks that the plain text password meets the minimum password requirements.
// It returns properly formatted errors for detailed form validation on the client.
func Validate(password string) error {
	// Ensure passwords are secure enough!

	// See: https://github.com/wagslane/go-password-validator#what-entropy-value-should-i-use
	err := passwordvalidator.Validate(password, 52)
	if err != nil {

		return err
	}
	if len(password) > 64 {
		return fmt.Errorf("password must be no more than %d characters", 64)
	}
	return nil
}
