package ctap2

import (
	"github.com/google/uuid"
	"github.com/coder/coder/v2/cli/fido2/internal/fido2/protocol/webauthn"
)

// Version represents the CTAP protocol version.
type Version string

const (
	FIDO_2_0     Version = "FIDO_2_0"
	FIDO_2_1_PRE Version = "FIDO_2_1_PRE"
	FIDO_2_1     Version = "FIDO_2_1"
	FIDO_2_2     Version = "FIDO_2_2"
	U2F_V2       Version = "U2F_V2"
)

// AuthenticatorGetInfoResponse represents the response to the AuthenticatorGetInfo command.
// It contains information about the authenticator's capabilities and configuration.
type AuthenticatorGetInfoResponse struct {
	Versions                         []Version                                `cbor:"1,keyasint"`
	Extensions                       []webauthn.ExtensionIdentifier           `cbor:"2,keyasint"`
	AAGUID                           uuid.UUID                                `cbor:"3,keyasint"`
	Options                          map[Option]bool                          `cbor:"4,keyasint"`
	MaxMsgSize                       uint                                     `cbor:"5,keyasint"`
	PinUvAuthProtocols               []PinUvAuthProtocolType                  `cbor:"6,keyasint"`
	MaxCredentialCountInList         uint                                     `cbor:"7,keyasint"`
	MaxCredentialLength              uint                                     `cbor:"8,keyasint"`
	Transports                       []string                                 `cbor:"9,keyasint"`
	Algorithms                       []webauthn.PublicKeyCredentialParameters `cbor:"10,keyasint"`
	MaxSerializedLargeBlobArray      uint                                     `cbor:"11,keyasint"`
	ForcePinChange                   bool                                     `cbor:"12,keyasint"`
	MinPinLength                     uint                                     `cbor:"13,keyasint"`
	FirmwareVersion                  uint                                     `cbor:"14,keyasint"`
	MaxCredBlobLength                uint                                     `cbor:"15,keyasint"`
	MaxRPIDsForSetMinPINLength       uint                                     `cbor:"16,keyasint"`
	PreferredPlatformUvAttempts      uint                                     `cbor:"17,keyasint"`
	UvModality                       uint                                     `cbor:"18,keyasint"`
	Certifications                   map[string]uint64                        `cbor:"19,keyasint"`
	RemainingDiscoverableCredentials uint                                     `cbor:"20,keyasint"`
	VendorPrototypeConfigCommands    []uint                                   `cbor:"21,keyasint"`
	AttestationFormats               []string                                 `cbor:"22,keyasint"`
	UvCountSinceLastPinEntry         uint                                     `cbor:"23,keyasint"`
	LongTouchForReset                bool                                     `cbor:"24,keyasint"`
	EncIdentifier                    string                                   `cbor:"25,keyasint"`
	TransportsForReset               []string                                 `cbor:"26,keyasint"`
	PinComplexityPolicy              bool                                     `cbor:"27,keyasint"`
	PinComplexityPolicyURL           string                                   `cbor:"28,keyasint"`
	MaxPINLength                     uint                                     `cbor:"29,keyasint"`
}

// IsPreviewOnly checks if the authenticator only supports FIDO 2.1 Preview version.
func (i AuthenticatorGetInfoResponse) IsPreviewOnly() bool {
	fidoTwo := false
	fidoTwoOnePre := false
	fidoTwoOne := false
	fidoTwoTwo := false

	for _, v := range i.Versions {
		switch v {
		case FIDO_2_0:
			fidoTwo = true
		case FIDO_2_1_PRE:
			fidoTwoOnePre = true
		case FIDO_2_1:
			fidoTwoOne = true
		case FIDO_2_2:
			fidoTwoTwo = true
		}
	}

	return fidoTwo && (!fidoTwoOne && !fidoTwoTwo && fidoTwoOnePre)
}
