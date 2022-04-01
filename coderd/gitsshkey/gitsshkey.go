package gitsshkey

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"

	"golang.org/x/xerrors"
)

type SSHKeygenAlgorithm string

const (
	// SSHKeygenAlgorithmEd25519 is the Edwards-curve Digital Signature Algorithm using Curve25519
	SSHKeygenAlgorithmEd25519 SSHKeygenAlgorithm = "ed25519"
	// SSHKeygenAlgorithmECDSA is the Digital Signature Algorithm (DSA) using NIST Elliptic Curve
	SSHKeygenAlgorithmECDSA SSHKeygenAlgorithm = "ecdsa"
	// SSHKeygenAlgorithmRSA4096 is the venerable Rivest-Shamir-Adleman algorithm
	// and creates a key with a fixed size of 4096-bit.
	SSHKeygenAlgorithmRSA4096 SSHKeygenAlgorithm = "rsa4096"
)

func GenerateKeyPair(algo SSHKeygenAlgorithm) ([]byte, []byte, error) {
	switch algo {
	case SSHKeygenAlgorithmEd25519:
		return ed25519KeyGen()
	case SSHKeygenAlgorithmECDSA:
		return ecdsaKeyGen()
	case SSHKeygenAlgorithmRSA4096:
		return rsa4096KeyGen()
	default:
		return nil, nil, xerrors.Errorf("invalid SSHKeygenAlgorithm: %s", algo)
	}
}

// ed25519KeyGen returns an ED25519-based SSH private key.
func ed25519KeyGen() ([]byte, []byte, error) {
	const blockType = "OPENSSH PRIVATE KEY"

	publicKey, privateKeyRaw, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, xerrors.Errorf("generate ed25519 private key: %w", err)
	}

	// NOTE: as of the time of writing, x/crypto/ssh is unable to marshal an ED25519 private key
	// into the format expected by OpenSSH. See: https://github.com/golang/go/issues/37132
	// Until this support is added, using a third-party implementation.
	byt, err := MarshalED25519PrivateKey(privateKeyRaw)
	if err != nil {
		return nil, nil, xerrors.Errorf("marshal ed25519 private key: %w", err)
	}

	pb := pem.Block{
		Type:    blockType,
		Headers: nil,
		Bytes:   byt,
	}
	privateKey := pem.EncodeToMemory(&pb)

	return privateKey, publicKey, nil
}

// ecdsaKeyGen returns an ECDSA-based SSH private key.
func ecdsaKeyGen() ([]byte, []byte, error) {
	const blockType = "EC PRIVATE KEY"

	privateKeyRaw, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, xerrors.Errorf("generate ecdsa private key: %w", err)
	}
	publicKey, err := x509.MarshalPKIXPublicKey(privateKeyRaw.PublicKey)
	if err != nil {
		return nil, nil, xerrors.Errorf("generate RSA4096 public key: %w", err)
	}

	byt, err := x509.MarshalECPrivateKey(privateKeyRaw)
	if err != nil {
		return nil, nil, xerrors.Errorf("marshal private key: %w", err)
	}

	pb := pem.Block{
		Type:    blockType,
		Headers: nil,
		Bytes:   byt,
	}
	privateKey := pem.EncodeToMemory(&pb)

	return privateKey, publicKey, nil
}

// rsaKeyGen returns an RSA-based SSH private key of size 4096.
//
// Administrators may configure this for SSH key compatibility with Azure DevOps.
func rsa4096KeyGen() ([]byte, []byte, error) {
	const blockType = "RSA PRIVATE KEY"

	privateKeyRaw, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, xerrors.Errorf("generate RSA4096 private key: %w", err)
	}
	publicKey, err := x509.MarshalPKIXPublicKey(privateKeyRaw.PublicKey)
	if err != nil {
		return nil, nil, xerrors.Errorf("generate RSA4096 public key: %w", err)
	}

	pb := pem.Block{
		Type:  blockType,
		Bytes: x509.MarshalPKCS1PrivateKey(privateKeyRaw),
	}
	privateKey := pem.EncodeToMemory(&pb)

	return privateKey, publicKey, nil
}

// ParseSSHKeygenAlgorithm returns a valid SSHKeygenAlgorithm or error if input is not a valid.
func ParseSSHKeygenAlgorithm(t string) (SSHKeygenAlgorithm, error) {
	ok := []string{
		string(SSHKeygenAlgorithmEd25519),
		string(SSHKeygenAlgorithmECDSA),
		string(SSHKeygenAlgorithmRSA4096),
	}

	for _, a := range ok {
		if string(t) == a {
			return SSHKeygenAlgorithm(a), nil
		}
	}

	return "", xerrors.Errorf(`invalid key type: %s, must be one of: %s`, t, strings.Join([]string(ok), ","))
}
