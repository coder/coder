package userpassword_test

import (
	"crypto/sha256"
	"testing"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/pbkdf2"

	"github.com/coder/coder/v2/cryptorand"
)

var (
	salt   = []byte(must(cryptorand.String(16)))
	secret = []byte(must(cryptorand.String(24)))

	resBcrypt []byte
	resPbkdf2 []byte
)

func BenchmarkBcryptMinCost(b *testing.B) {
	var r []byte
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r, _ = bcrypt.GenerateFromPassword(secret, bcrypt.MinCost)
	}

	resBcrypt = r
}

func BenchmarkPbkdf2MinCost(b *testing.B) {
	var r []byte
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r = pbkdf2.Key(secret, salt, 1024, 64, sha256.New)
	}

	resPbkdf2 = r
}

func BenchmarkBcryptDefaultCost(b *testing.B) {
	var r []byte
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r, _ = bcrypt.GenerateFromPassword(secret, bcrypt.DefaultCost)
	}

	resBcrypt = r
}

func BenchmarkPbkdf2(b *testing.B) {
	var r []byte
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r = pbkdf2.Key(secret, salt, 65536, 64, sha256.New)
	}

	resPbkdf2 = r
}

func must(s string, err error) string {
	if err != nil {
		panic(err)
	}

	return s
}
