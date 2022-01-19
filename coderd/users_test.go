package coderd_test

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"testing"

	"golang.org/x/crypto/pbkdf2"
)

func TestUsers(t *testing.T) {
	// t.Run("AuthenticatedUser", func(t *testing.T) {
	// 	_ = coderdtest.New(t)
	// })

	t.Run("Pass", func(t *testing.T) {
		salt := make([]byte, 16)
		_, err := rand.Read(salt)
		if err != nil {
			panic("unexpected: crypto/rand.Read returned an error: " + err.Error())
		}
		hash := pbkdf2.Key([]byte("hello"), salt, 65535, 64, sha256.New)
		str := base64.StdEncoding.EncodeToString(hash)

		parts := bytes.Split(hash, []byte("$"))
		fmt.Printf("Parts: %+v\n", len(parts))

		fmt.Printf("Hash: %s %s\n", hash, str)
	})
}
