package ctap2

import "github.com/ldclabs/cose/key"

// AuthenticatorClientPINRequest represents the request for AuthenticatorClientPIN command.
type AuthenticatorClientPINRequest struct {
	PinUvAuthProtocol PinUvAuthProtocolType `cbor:"1,keyasint,omitzero"`
	SubCommand        ClientPINSubCommand   `cbor:"2,keyasint"`
	KeyAgreement      key.Key               `cbor:"3,keyasint,omitzero"`
	PinUvAuthParam    []byte                `cbor:"4,keyasint,omitempty"`
	NewPinEnc         []byte                `cbor:"5,keyasint,omitempty"`
	PinHashEnc        []byte                `cbor:"6,keyasint,omitempty"`
	Permissions       Permission            `cbor:"9,keyasint,omitempty"`
	RPID              string                `cbor:"10,keyasint,omitempty"`
}

// AuthenticatorClientPINResponse represents the response for AuthenticatorClientPIN command.
type AuthenticatorClientPINResponse struct {
	KeyAgreement    key.Key `cbor:"1,keyasint"`
	PinUvAuthToken  []byte  `cbor:"2,keyasint"`
	PinRetries      uint    `cbor:"3,keyasint"`
	PowerCycleState bool    `cbor:"4,keyasint"`
	UvRetries       uint    `cbor:"5,keyasint"`
}

// ClientPINSubCommand represents the sub-command for AuthenticatorClientPIN.
type ClientPINSubCommand byte

func (cmd ClientPINSubCommand) String() string {
	return clientPINSubCommandStringMap[cmd]
}

const (
	// ClientPINSubCommandGetPINRetries retrieves the number of PIN retries remaining.
	ClientPINSubCommandGetPINRetries ClientPINSubCommand = iota + 1
	// ClientPINSubCommandGetKeyAgreement retrieves the key agreement key.
	ClientPINSubCommandGetKeyAgreement
	// ClientPINSubCommandSetPIN sets the PIN.
	ClientPINSubCommandSetPIN
	// ClientPINSubCommandChangePIN changes the PIN.
	ClientPINSubCommandChangePIN
	// ClientPINSubCommandGetPinToken retrieves the PIN token.
	ClientPINSubCommandGetPinToken
	// ClientPINSubCommandGetPinUvAuthTokenUsingUvWithPermissions retrieves the PIN/UV auth token using UV.
	ClientPINSubCommandGetPinUvAuthTokenUsingUvWithPermissions
	// ClientPINSubCommandGetUVRetries retrieves the number of UV retries remaining.
	ClientPINSubCommandGetUVRetries
	_ // Reserved
	// ClientPINSubCommandGetPinUvAuthTokenUsingPinWithPermissions retrieves the PIN/UV auth token using PIN.
	ClientPINSubCommandGetPinUvAuthTokenUsingPinWithPermissions
)

var clientPINSubCommandStringMap = map[ClientPINSubCommand]string{
	ClientPINSubCommandGetPINRetries:                            "GetPINRetries",
	ClientPINSubCommandGetKeyAgreement:                          "GetKeyAgreement",
	ClientPINSubCommandSetPIN:                                   "SetPIN",
	ClientPINSubCommandChangePIN:                                "ChangePIN",
	ClientPINSubCommandGetPinToken:                              "GetPinToken",
	ClientPINSubCommandGetPinUvAuthTokenUsingUvWithPermissions:  "GetPinUvAuthTokenUsingUvWithPermissions",
	ClientPINSubCommandGetUVRetries:                             "GetUVRetries",
	ClientPINSubCommandGetPinUvAuthTokenUsingPinWithPermissions: "GetPinUvAuthTokenUsingPinWithPermissions",
}

// Permission represents permissions for PinUvAuthToken.
type Permission byte

func (p Permission) String() string {
	if str, ok := permissionStringMap[p]; ok {
		return str
	}
	return "Unknown Permission"
}

const (
	// PermissionNone represents no permissions.
	PermissionNone Permission = 0x00
	// PermissionMakeCredential represents permission to make a credential.
	PermissionMakeCredential Permission = 0x01
	// PermissionGetAssertion represents permission to get an assertion.
	PermissionGetAssertion Permission = 0x02
	// PermissionCredentialManagement represents permission for credential management.
	PermissionCredentialManagement Permission = 0x04
	// PermissionBioEnrollment represents permission for biometric enrollment.
	PermissionBioEnrollment Permission = 0x08
	// PermissionLargeBlobWrite represents permission to write large blobs.
	PermissionLargeBlobWrite Permission = 0x10
	// PermissionAuthenticatorConfiguration represents permission for authenticator configuration.
	PermissionAuthenticatorConfiguration Permission = 0x20
	// PermissionPersistentCredentialManagementReadOnly represents permission for read-only credential management.
	PermissionPersistentCredentialManagementReadOnly Permission = 0x40
)

var permissionStringMap = map[Permission]string{
	PermissionNone:                                   "None",
	PermissionMakeCredential:                         "Make Credential",
	PermissionGetAssertion:                           "Get Assertion",
	PermissionCredentialManagement:                   "Credential Management",
	PermissionBioEnrollment:                          "Bio Enrollment",
	PermissionLargeBlobWrite:                         "Large Blob Write",
	PermissionAuthenticatorConfiguration:             "Authenticator Configuration",
	PermissionPersistentCredentialManagementReadOnly: "Persistent Credential Management Read Only",
}
