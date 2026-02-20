package ctap2

// Command represents a CTAP2 command code.
type Command byte

func (cmd Command) String() string {
	return commandStringMap[cmd]
}

// CTAP2 Command Codes.
const (
	// CMDAuthenticatorMakeCredential creates a new credential.
	CMDAuthenticatorMakeCredential Command = 0x01
	// CMDAuthenticatorGetAssertion retrieves an assertion.
	CMDAuthenticatorGetAssertion Command = 0x02
	// CMDAuthenticatorGetNextAssertion retrieves the next assertion in a series.
	CMDAuthenticatorGetNextAssertion Command = 0x08
	// CMDAuthenticatorGetInfo retrieves authenticator information.
	CMDAuthenticatorGetInfo Command = 0x04
	// CMDAuthenticatorClientPIN manages the PIN.
	CMDAuthenticatorClientPIN Command = 0x06
	// CMDAuthenticatorReset performs a factory reset.
	CMDAuthenticatorReset Command = 0x07
	// CMDAuthenticatorBioEnrollment manages biometric enrollment.
	CMDAuthenticatorBioEnrollment Command = 0x09
	// CMDAuthenticatorCredentialManagement manages credentials.
	CMDAuthenticatorCredentialManagement Command = 0x0a
	// CMDAuthenticatorSelection performs account selection.
	CMDAuthenticatorSelection Command = 0x0b
	// CMDAuthenticatorLargeBlobs manages large blobs.
	CMDAuthenticatorLargeBlobs Command = 0x0c
	// CMDAuthenticatorConfig configures the authenticator.
	CMDAuthenticatorConfig Command = 0x0d
	// CMDPrototypeAuthenticatorBioEnrollment is a prototype command for biometric enrollment.
	CMDPrototypeAuthenticatorBioEnrollment Command = 0x40
	// CMDPrototypeAuthenticatorCredentialManagement is a prototype command for credential management.
	CMDPrototypeAuthenticatorCredentialManagement Command = 0x41
)

var commandStringMap = map[Command]string{
	CMDAuthenticatorMakeCredential:                "AuthenticatorMakeCredential",
	CMDAuthenticatorGetAssertion:                  "AuthenticatorGetAssertion",
	CMDAuthenticatorGetNextAssertion:              "AuthenticatorGetNextAssertion",
	CMDAuthenticatorGetInfo:                       "AuthenticatorGetInfo",
	CMDAuthenticatorClientPIN:                     "AuthenticatorClientPIN",
	CMDAuthenticatorReset:                         "AuthenticatorReset",
	CMDAuthenticatorBioEnrollment:                 "AuthenticatorBioEnrollment",
	CMDAuthenticatorCredentialManagement:          "AuthenticatorCredentialManagement",
	CMDAuthenticatorSelection:                     "AuthenticatorSelection",
	CMDAuthenticatorLargeBlobs:                    "AuthenticatorLargeBlobs",
	CMDAuthenticatorConfig:                        "AuthenticatorConfig",
	CMDPrototypeAuthenticatorBioEnrollment:        "PrototypeAuthenticatorBioEnrollment",
	CMDPrototypeAuthenticatorCredentialManagement: "PrototypeAuthenticatorCredentialManagement",
}

// Option represents a CTAP2 option key.
type Option string

func (o Option) String() string {
	return optionStringMap[o]
}

// CTAP2 Options.
const (
	// OptionPlatformDevice means the authenticator is a platform device.
	OptionPlatformDevice Option = "plat"
	// OptionResidentKeys means the authenticator supports resident keys.
	OptionResidentKeys Option = "rk"
	// OptionClientPIN means the authenticator supports client PIN.
	OptionClientPIN Option = "clientPin"
	// OptionUserPresence means the authenticator supports user presence.
	OptionUserPresence Option = "up"
	// OptionUserVerification means the authenticator supports user verification.
	OptionUserVerification Option = "uv"
	// OptionPinUvAuthToken means the authenticator supports PIN/UV auth token.
	OptionPinUvAuthToken Option = "pinUvAuthToken"
	// OptionNoMcGaPermissionsWithClientPin means no McGa permissions with client PIN.
	OptionNoMcGaPermissionsWithClientPin Option = "noMcGaPermissionsWithClientPin"
	// OptionLargeBlobs means the authenticator supports large blobs.
	OptionLargeBlobs Option = "largeBlobs"
	// OptionEnterpriseAttestation means the authenticator supports enterprise attestation.
	OptionEnterpriseAttestation Option = "ep"
	// OptionBioEnroll means the authenticator supports biometric enrollment.
	OptionBioEnroll Option = "bioEnroll"
	// OptionUserVerificationMgmtPreview means the authenticator supports user verification management preview.
	OptionUserVerificationMgmtPreview Option = "userVerificationMgmtPreview"
	// OptionUvBioEnroll means the authenticator supports UV biometric enrollment.
	OptionUvBioEnroll Option = "uvBioEnroll"
	// OptionAuthenticatorConfig means the authenticator supports authenticator configuration.
	OptionAuthenticatorConfig Option = "authnrCfg"
	// OptionUvAcfg means the authenticator supports UV authenticator configuration.
	OptionUvAcfg Option = "uvAcfg"
	// OptionCredentialManagement means the authenticator supports credential management.
	OptionCredentialManagement Option = "credMgmt"
	// OptionCredentialManagementReadOnly means the authenticator supports read-only credential management.
	OptionCredentialManagementReadOnly Option = "perCredMgmtRO"
	// OptionCredentialManagementPreview means the authenticator supports credential management preview.
	OptionCredentialManagementPreview Option = "credentialMgmtPreview"
	// OptionSetMinPINLength means the authenticator supports setting minimum PIN length.
	OptionSetMinPINLength Option = "setMinPINLength"
	// OptionMakeCredentialUvNotRequired means user verification is not required for MakeCredential.
	OptionMakeCredentialUvNotRequired Option = "makeCredUvNotRqd"
	// OptionAlwaysUv means user verification is always required.
	OptionAlwaysUv Option = "alwaysUv"
)

var optionStringMap = map[Option]string{
	OptionPlatformDevice:                 "Platform Device",
	OptionResidentKeys:                   "Resident Keys",
	OptionClientPIN:                      "Client PIN",
	OptionUserPresence:                   "User Presence",
	OptionUserVerification:               "User Verification",
	OptionPinUvAuthToken:                 "PIN/UV Auth Token",
	OptionNoMcGaPermissionsWithClientPin: "No McGa Permissions With Client PIN",
	OptionLargeBlobs:                     "Large Blobs",
	OptionEnterpriseAttestation:          "Enterprise Attestation",
	OptionBioEnroll:                      "Bio Enroll",
	OptionUserVerificationMgmtPreview:    "User Verification Management Preview",
	OptionUvBioEnroll:                    "UV Bio Enroll",
	OptionAuthenticatorConfig:            "Authenticator Configuration",
	OptionUvAcfg:                         "UV Acfg",
	OptionCredentialManagement:           "Credential Management",
	OptionCredentialManagementReadOnly:   "Credential Management Read Only",
	OptionCredentialManagementPreview:    "Credential Management Preview",
	OptionSetMinPINLength:                "Set Minimum PIN Length",
	OptionMakeCredentialUvNotRequired:    "Make Credential UV Not Required",
	OptionAlwaysUv:                       "Always UV",
}
