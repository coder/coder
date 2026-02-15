package ctap2

import (
	"crypto/ecdh"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/coder/coder/v2/cli/fido2/internal/fido2/protocol/ctap2/pin/protocolone"
	"github.com/coder/coder/v2/cli/fido2/internal/fido2/protocol/ctap2/pin/protocoltwo"
	"github.com/ldclabs/cose/iana"
	"github.com/ldclabs/cose/key"
	ecdh2 "github.com/ldclabs/cose/key/ecdh"
)

var (
	// ErrInvalidPinAuthProtocol is returned when an unsupported PIN/UV auth protocol is requested.
	ErrInvalidPinAuthProtocol = errors.New("invalid auth protocol")
)

// PinUvAuthProtocolType represents the PIN/UV auth protocol version.
type PinUvAuthProtocolType uint

func (p PinUvAuthProtocolType) String() string {
	return PinUvAuthProtocolStringMap[p]
}

const (
	// PinUvAuthProtocolTypeOne is PIN/UV auth protocol version 1.
	PinUvAuthProtocolTypeOne PinUvAuthProtocolType = iota + 1
	// PinUvAuthProtocolTypeTwo is PIN/UV auth protocol version 2.
	PinUvAuthProtocolTypeTwo
)

// PinUvAuthProtocolStringMap maps PIN/UV auth protocol types to their string representations.
var PinUvAuthProtocolStringMap = map[PinUvAuthProtocolType]string{
	PinUvAuthProtocolTypeOne: "PinUvAuthProtocolOne",
	PinUvAuthProtocolTypeTwo: "PinUvAuthProtocolTwo",
}

// PinUvAuthProtocol handles the cryptographic operations for PIN/UV authentication.
type PinUvAuthProtocol struct {
	Type               PinUvAuthProtocolType
	platformPrivateKey *ecdh.PrivateKey
	platformCoseKey    key.Key
}

// NewPinUvAuthProtocol creates a new PinUvAuthProtocol instance.
func NewPinUvAuthProtocol(number PinUvAuthProtocolType) (*PinUvAuthProtocol, error) {
	platformPrivkey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("cannot generate platform P-256 keypair: %w", err)
	}

	platformPubkey, err := ecdh2.KeyFromPublic(platformPrivkey.Public().(*ecdh.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("cannot convert platform public key to COSE_Key: %w", err)
	}
	if err := platformPubkey.Set(iana.KeyParameterAlg, -25); err != nil {
		return nil, fmt.Errorf("cannot set alg parameter for COSE_Key: %w", err)
	}

	// Specification explicitly requires COSE_Key to contain only the necessary parameters.
	// Some keys accept it anyway, but some are not, e.g., SoloKeys Solo 2.
	delete(platformPubkey, iana.KeyParameterKid)

	return &PinUvAuthProtocol{
		Type:               number,
		platformPrivateKey: platformPrivkey,
		platformCoseKey:    platformPubkey,
	}, nil
}

// ECDH performs Elliptic Curve Diffie-Hellman to derive a shared secret.
func (p *PinUvAuthProtocol) ECDH(peerCoseKey key.Key) ([]byte, error) {
	peerPubkey, err := ecdh2.KeyToPublic(peerCoseKey)
	if err != nil {
		return nil, fmt.Errorf("cannot convert peer public key to Go *ecdh.PublicKey: %w", err)
	}

	sharedSecret, err := p.platformPrivateKey.ECDH(peerPubkey)
	if err != nil {
		return nil, fmt.Errorf("cannot derive shared secret: %w", err)
	}

	return p.KDF(sharedSecret)
}

// KDF derives a key from the shared secret using the appropriate protocol KDF.
func (p *PinUvAuthProtocol) KDF(z []byte) ([]byte, error) {
	switch p.Type {
	case PinUvAuthProtocolTypeOne:
		return protocolone.KDF(z), nil
	case PinUvAuthProtocolTypeTwo:
		return protocoltwo.KDF(z)
	default:
		return nil, ErrInvalidPinAuthProtocol
	}
}

// Encrypt encrypts the plaintext using the shared secret and appropriate protocol encryption.
func (p *PinUvAuthProtocol) Encrypt(sharedSecret []byte, demPlaintext []byte) ([]byte, error) {
	switch p.Type {
	case PinUvAuthProtocolTypeOne:
		return protocolone.Encrypt(sharedSecret, demPlaintext)
	case PinUvAuthProtocolTypeTwo:
		return protocoltwo.Encrypt(sharedSecret, demPlaintext)
	default:
		return nil, ErrInvalidPinAuthProtocol
	}
}

// Decrypt decrypts the ciphertext using the shared secret and appropriate protocol decryption.
func (p *PinUvAuthProtocol) Decrypt(sharedSecret []byte, demCiphertext []byte) ([]byte, error) {
	switch p.Type {
	case PinUvAuthProtocolTypeOne:
		return protocolone.Decrypt(sharedSecret, demCiphertext)
	case PinUvAuthProtocolTypeTwo:
		return protocoltwo.Decrypt(sharedSecret, demCiphertext)
	default:
		return nil, ErrInvalidPinAuthProtocol
	}
}

// Encapsulate performs key agreement and returns the platform key and shared secret.
func (p *PinUvAuthProtocol) Encapsulate(peerCoseKey key.Key) (key.Key, []byte, error) {
	sharedSecret, err := p.ECDH(peerCoseKey)
	if err != nil {
		return nil, nil, err
	}

	return p.platformCoseKey, sharedSecret, nil
}

// Authenticate calculates the authentication MAC for the message.
func Authenticate(number PinUvAuthProtocolType, sharedSecret []byte, message []byte) []byte {
	switch number {
	case PinUvAuthProtocolTypeOne:
		return protocolone.Authenticate(sharedSecret, message)
	case PinUvAuthProtocolTypeTwo:
		return protocoltwo.Authenticate(sharedSecret, message)
	default:
		return nil
	}
}

// AuthenticateWithError calculates the authentication MAC for the message, returning an error
// when an unsupported PIN/UV auth protocol is requested.
func AuthenticateWithError(number PinUvAuthProtocolType, sharedSecret []byte, message []byte) ([]byte, error) {
	mac := Authenticate(number, sharedSecret, message)
	if mac == nil {
		return nil, ErrInvalidPinAuthProtocol
	}
	return mac, nil
}
