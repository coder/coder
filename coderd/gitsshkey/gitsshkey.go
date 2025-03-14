package gitsshkey
import (
	"fmt"
	"errors"
	"bufio"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"io"
	"strings"
	"time"
	insecurerand "math/rand"
	"golang.org/x/crypto/ssh"
)
type Algorithm string
const (
	// AlgorithmEd25519 is the Edwards-curve Digital Signature Algorithm using Curve25519
	AlgorithmEd25519 Algorithm = "ed25519"
	// AlgorithmECDSA is the Digital Signature Algorithm (DSA) using NIST Elliptic Curve
	AlgorithmECDSA Algorithm = "ecdsa"
	// AlgorithmRSA4096 is the venerable Rivest-Shamir-Adleman algorithm
	// and creates a key with a fixed size of 4096-bit.
	AlgorithmRSA4096 Algorithm = "rsa4096"
)
func entropy() io.Reader {
	if flag.Lookup("test.v") != nil {
		// This helps speed along our tests, esp. in CI where entropy is
		// sparse.
		//nolint:gosec
		return insecurerand.New(insecurerand.NewSource(time.Now().UnixNano()))
	}
	// Buffering to reduce the number of system calls
	// doubles performance without any loss of security.
	return bufio.NewReader(rand.Reader)
}
// ParseAlgorithm returns a valid Algorithm or error if input is not a valid.
func ParseAlgorithm(t string) (Algorithm, error) {
	ok := []string{
		string(AlgorithmEd25519),
		string(AlgorithmECDSA),
		string(AlgorithmRSA4096),
	}
	for _, a := range ok {
		if strings.EqualFold(a, t) {
			return Algorithm(a), nil
		}
	}
	return "", fmt.Errorf(`invalid key type: %s, must be one of: %s`, t, strings.Join(ok, ","))
}
// Generate creates a private key in the OpenSSH PEM format and public key in
// the authorized key format.
func Generate(algo Algorithm) (privateKey string, publicKey string, err error) {
	switch algo {
	case AlgorithmEd25519:
		return ed25519KeyGen()
	case AlgorithmECDSA:
		return ecdsaKeyGen()
	case AlgorithmRSA4096:
		return rsa4096KeyGen()
	default:
		return "", "", fmt.Errorf("invalid algorithm: %s", algo)
	}
}
// ed25519KeyGen returns an ED25519-based SSH private key.
func ed25519KeyGen() (privateKey string, publicKey string, err error) {
	_, privateKeyRaw, err := ed25519.GenerateKey(entropy())
	if err != nil {
		return "", "", fmt.Errorf("generate ed25519 private key: %w", err)
	}
	// NOTE: as of the time of writing, x/crypto/ssh is unable to marshal an ED25519 private key
	// into the format expected by OpenSSH. See: https://github.com/golang/go/issues/37132
	// Until this support is added, using a third-party implementation.
	byt, err := MarshalED25519PrivateKey(privateKeyRaw)
	if err != nil {
		return "", "", fmt.Errorf("marshal ed25519 private key: %w", err)
	}
	return generateKeys(pem.Block{
		Type:  "OPENSSH PRIVATE KEY",
		Bytes: byt,
	}, privateKeyRaw)
}
// ecdsaKeyGen returns an ECDSA-based SSH private key.
func ecdsaKeyGen() (privateKey string, publicKey string, err error) {
	privateKeyRaw, err := ecdsa.GenerateKey(elliptic.P256(), entropy())
	if err != nil {
		return "", "", fmt.Errorf("generate ecdsa private key: %w", err)
	}
	byt, err := x509.MarshalECPrivateKey(privateKeyRaw)
	if err != nil {
		return "", "", fmt.Errorf("marshal private key: %w", err)
	}
	return generateKeys(pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: byt,
	}, privateKeyRaw)
}
// rsaKeyGen returns an RSA-based SSH private key of size 4096.
//
// Administrators may configure this for SSH key compatibility with Azure DevOps.
func rsa4096KeyGen() (privateKey string, publicKey string, err error) {
	privateKeyRaw, err := rsa.GenerateKey(entropy(), 4096)
	if err != nil {
		return "", "", fmt.Errorf("generate RSA4096 private key: %w", err)
	}
	return generateKeys(pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKeyRaw),
	}, privateKeyRaw)
}
func generateKeys(block pem.Block, cp crypto.Signer) (privateKey string, publicKey string, err error) {
	pkBytes := pem.EncodeToMemory(&block)
	privateKey = string(pkBytes)
	publicKeyRaw := cp.Public()
	p, err := ssh.NewPublicKey(publicKeyRaw)
	if err != nil {
		return "", "", err
	}
	publicKey = string(ssh.MarshalAuthorizedKey(p))
	return privateKey, publicKey, nil
}
