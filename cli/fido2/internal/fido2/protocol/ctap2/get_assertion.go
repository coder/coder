package ctap2

import (
	"bytes"
	"encoding/binary"
	"errors"

	"github.com/fxamacker/cbor/v2"
	"github.com/google/uuid"
	"github.com/ldclabs/cose/key"
	"github.com/coder/coder/v2/cli/fido2/internal/fido2/protocol/webauthn"
)

// AuthDataFlag represents flags in the Authenticator Data.
type AuthDataFlag byte

const (
	AuthDataFlagUserPresent AuthDataFlag = 1 << iota
	_
	AuthDataFlagUserVerified
	_
	_
	_
	AuthDataFlagAttestedCredentialDataIncluded
	AuthDataFlagExtensionDataIncluded
)

// UserPresent checks if the User Present flag is set.
func (f AuthDataFlag) UserPresent() bool {
	return f&AuthDataFlagUserPresent != 0
}

// UserVerified checks if the User Verified flag is set.
func (f AuthDataFlag) UserVerified() bool {
	return f&AuthDataFlagUserVerified != 0
}

// AttestedCredentialDataIncluded checks if the Attested Credential Data Included flag is set.
func (f AuthDataFlag) AttestedCredentialDataIncluded() bool {
	return f&AuthDataFlagAttestedCredentialDataIncluded != 0
}

// ExtensionDataIncluded checks if the Extension Data Included flag is set.
func (f AuthDataFlag) ExtensionDataIncluded() bool {
	return f&AuthDataFlagExtensionDataIncluded != 0
}

// AttestedCredentialData contains credential data.
type AttestedCredentialData struct {
	AAGUID              uuid.UUID
	CredentialID        []byte
	CredentialPublicKey key.Key
}

// AuthenticatorGetAssertionRequest represents the request for AuthenticatorGetAssertion command.
type AuthenticatorGetAssertionRequest struct {
	RPID              string                                   `cbor:"1,keyasint"`
	ClientDataHash    []byte                                   `cbor:"2,keyasint"`
	AllowList         []webauthn.PublicKeyCredentialDescriptor `cbor:"3,keyasint,omitempty"`
	Extensions        *GetExtensionInputs                      `cbor:"4,keyasint,omitempty"`
	Options           map[Option]bool                          `cbor:"5,keyasint,omitempty"`
	PinUvAuthParam    []byte                                   `cbor:"6,keyasint,omitempty"`
	PinUvAuthProtocol PinUvAuthProtocolType                    `cbor:"7,keyasint,omitempty"`
}

// AuthenticatorGetAssertionResponse represents the response for AuthenticatorGetAssertion command.
type AuthenticatorGetAssertionResponse struct {
	Credential               webauthn.PublicKeyCredentialDescriptor             `cbor:"1,keyasint"`
	AuthDataRaw              []byte                                             `cbor:"2,keyasint"`
	AuthData                 *GetAssertionAuthData                              `cbor:"-"`
	Signature                []byte                                             `cbor:"3,keyasint"`
	User                     *webauthn.PublicKeyCredentialUserEntity            `cbor:"4,keyasint,omitempty"`
	NumberOfCredentials      uint                                               `cbor:"5,keyasint,omitempty"`
	UserSelected             bool                                               `cbor:"6,keyasint,omitempty"`
	LargeBlobKey             []byte                                             `cbor:"7,keyasint,omitempty"`
	UnsignedExtensionOutputs map[webauthn.ExtensionIdentifier]any               `cbor:"8,keyasint,omitempty"`
	ExtensionOutputs         *webauthn.GetAuthenticationExtensionsClientOutputs `cbor:"-"`
}

// GetAssertionAuthData represents the authenticator data returned in GetAssertion response.
type GetAssertionAuthData struct {
	RPIDHash               []byte
	Flags                  AuthDataFlag
	SignCount              uint32
	AttestedCredentialData *AttestedCredentialData
	Extensions             *GetExtensionOutputs
}

// ParseGetAssertionAuthData parses the authenticator data for GetAssertion.
func ParseGetAssertionAuthData(data []byte) (*GetAssertionAuthData, error) {
	d, err := parseAuthData(data)
	if err != nil {
		return nil, err
	}

	getAssertionAuthData := &GetAssertionAuthData{
		RPIDHash:               d.RPIDHash,
		Flags:                  d.Flags,
		SignCount:              d.SignCount,
		AttestedCredentialData: d.AttestedCredentialData,
	}

	if d.Extensions != nil {
		if err := cbor.NewDecoder(bytes.NewReader(d.Extensions)).
			Decode(&getAssertionAuthData.Extensions); err != nil {
			return nil, err
		}
	}

	return getAssertionAuthData, nil
}

type authData struct {
	RPIDHash               []byte
	Flags                  AuthDataFlag
	SignCount              uint32
	AttestedCredentialData *AttestedCredentialData
	Extensions             []byte
}

var errAuthDataTooShort = errors.New("ctap2: authData too short")

func parseAuthData(data []byte) (*authData, error) {
	if len(data) < 37 {
		return nil, errAuthDataTooShort
	}
	d := &authData{
		RPIDHash:  data[:32],
		Flags:     AuthDataFlag(data[32]),
		SignCount: binary.BigEndian.Uint32(data[33:37]),
	}
	offset := 37
	if d.Flags.AttestedCredentialDataIncluded() {
		if len(data) < offset+16+2 {
			return nil, errAuthDataTooShort
		}
		credData := &AttestedCredentialData{
			AAGUID: uuid.UUID(data[offset : offset+16]),
		}
		offset += 16

		// Credential ID
		length := binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2
		if len(data) < offset+int(length) {
			return nil, errAuthDataTooShort
		}
		credData.CredentialID = data[offset : offset+int(length)]
		offset += int(length)

		// Credential Public Key
		dec := cbor.NewDecoder(bytes.NewReader(data[offset:]))
		if err := dec.Decode(&credData.CredentialPublicKey); err != nil {
			return nil, err
		}
		offset += dec.NumBytesRead()
		if offset > len(data) {
			return nil, errAuthDataTooShort
		}

		d.AttestedCredentialData = credData
	}

	if d.Flags.ExtensionDataIncluded() {
		if offset > len(data) {
			return nil, errAuthDataTooShort
		}
		d.Extensions = data[offset:]
	}

	return d, nil
}
