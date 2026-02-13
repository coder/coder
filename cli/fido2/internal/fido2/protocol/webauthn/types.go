package webauthn

import "github.com/ldclabs/cose/key"

type (
	// PublicKeyCredentialType defines the valid credential types.
	// https://www.w3.org/TR/webauthn-3/#enumdef-publickeycredentialtype
	PublicKeyCredentialType string
	// AuthenticatorTransport defines hints as to how clients might communicate
	// with a particular authenticator in order to obtain an assertion for a specific credential.
	// https://www.w3.org/TR/webauthn-3/#enumdef-authenticatortransport
	AuthenticatorTransport string
	// AttestationStatementFormatIdentifier is an enum consisting of IANA registered Attestation Statement Format Identifiers.
	// https://www.iana.org/assignments/webauthn/webauthn.xhtml
	AttestationStatementFormatIdentifier string
	// ExtensionIdentifier is an enum consisting of IANA registered Extension Identifiers.
	// https://www.iana.org/assignments/webauthn/webauthn.xhtml
	ExtensionIdentifier string
	// PublicKeyCredentialHint is used by WebAuthn Relying Parties to communicate hints to the user-agent about
	// how a request may be best completed.
	// https://www.w3.org/TR/webauthn-3/#enum-hints
	PublicKeyCredentialHint string
)

const (
	// PublicKeyCredentialTypePublicKey is the only supported credential type.
	PublicKeyCredentialTypePublicKey PublicKeyCredentialType = "public-key"
)

const (
	// AuthenticatorTransportUSB means the authenticator can be reached via USB.
	AuthenticatorTransportUSB AuthenticatorTransport = "usb"
	// AuthenticatorTransportNFC means the authenticator can be reached via NFC.
	AuthenticatorTransportNFC AuthenticatorTransport = "nfc"
	// AuthenticatorTransportBLE means the authenticator can be reached via BLE.
	AuthenticatorTransportBLE AuthenticatorTransport = "ble"
	// AuthenticatorTransportSmartCard means the authenticator can be reached via Smart Card.
	AuthenticatorTransportSmartCard AuthenticatorTransport = "smart-card"
	// AuthenticatorTransportHybrid means the authenticator can be reached via Hybrid transport.
	AuthenticatorTransportHybrid AuthenticatorTransport = "hybrid"
	// AuthenticatorTransportInternal means the authenticator is internal to the client device.
	AuthenticatorTransportInternal AuthenticatorTransport = "internal"
)

const (
	// AttestationStatementFormatIdentifierPacked is the Packed attestation statement format.
	AttestationStatementFormatIdentifierPacked AttestationStatementFormatIdentifier = "packed"
	// AttestationStatementFormatIdentifierTPM is the TPM attestation statement format.
	AttestationStatementFormatIdentifierTPM AttestationStatementFormatIdentifier = "tpm"
	// AttestationStatementFormatIdentifierAndroidKey is the Android Key attestation statement format.
	AttestationStatementFormatIdentifierAndroidKey AttestationStatementFormatIdentifier = "android-key"
	// AttestationStatementFormatIdentifierAndroidSafetyNet is the Android SafetyNet attestation statement format.
	AttestationStatementFormatIdentifierAndroidSafetyNet AttestationStatementFormatIdentifier = "android-safetynet"
	// AttestationStatementFormatIdentifierFIDOU2F is the FIDO U2F attestation statement format.
	AttestationStatementFormatIdentifierFIDOU2F AttestationStatementFormatIdentifier = "fido-u2f"
	// AttestationStatementFormatIdentifierApple is the Apple attestation statement format.
	AttestationStatementFormatIdentifierApple AttestationStatementFormatIdentifier = "apple"
	// AttestationStatementFormatIdentifierNone is the None attestation statement format.
	AttestationStatementFormatIdentifierNone AttestationStatementFormatIdentifier = "none"
)

const (
	// ExtensionIdentifierAppID is the FIDO AppID extension.
	ExtensionIdentifierAppID ExtensionIdentifier = "appid"
	// ExtensionIdentifierTxAuthSimple is the Simple Transaction Authorization extension.
	ExtensionIdentifierTxAuthSimple ExtensionIdentifier = "txAuthSimple"
	// ExtensionIdentifierTxAuthGeneric is the Generic Transaction Authorization extension.
	ExtensionIdentifierTxAuthGeneric ExtensionIdentifier = "txAuthGeneric"
	// ExtensionIdentifierAuthnSelection is the Authenticator Selection extension.
	ExtensionIdentifierAuthnSelection ExtensionIdentifier = "authnSel"
	// ExtensionIdentifierExtensions is the Client Extension Results extension.
	ExtensionIdentifierExtensions ExtensionIdentifier = "exts"
	// ExtensionIdentifierUserVerificationIndex is the User Verification Index extension.
	ExtensionIdentifierUserVerificationIndex ExtensionIdentifier = "uvi"
	// ExtensionIdentifierLocation is the Location extension.
	ExtensionIdentifierLocation ExtensionIdentifier = "loc"
	// ExtensionIdentifierUserVerificationMethod is the User Verification Method extension.
	ExtensionIdentifierUserVerificationMethod ExtensionIdentifier = "uvm"
	// ExtensionIdentifierCredentialProtection is the Credential Protection extension.
	ExtensionIdentifierCredentialProtection ExtensionIdentifier = "credProtect"
	// ExtensionIdentifierCredentialBlob is the Credential Blob extension.
	ExtensionIdentifierCredentialBlob ExtensionIdentifier = "credBlob"
	// ExtensionIdentifierLargeBlobKey is the Large Blob Key extension.
	ExtensionIdentifierLargeBlobKey ExtensionIdentifier = "largeBlobKey"
	// ExtensionIdentifierMinPinLength is the Minimum PIN Length extension.
	ExtensionIdentifierMinPinLength ExtensionIdentifier = "minPinLength"
	// ExtensionIdentifierPinComplexityPolicy is the PIN Complexity Policy extension.
	ExtensionIdentifierPinComplexityPolicy ExtensionIdentifier = "pinComplexityPolicy"
	// ExtensionIdentifierHMACSecret is the HMAC Secret extension.
	ExtensionIdentifierHMACSecret ExtensionIdentifier = "hmac-secret"
	// ExtensionIdentifierHMACSecretMC is the HMAC Secret extension for credential creation.
	ExtensionIdentifierHMACSecretMC ExtensionIdentifier = "hmac-secret-mc"
	// ExtensionIdentifierAppIDExclude is the AppID Exclude extension.
	ExtensionIdentifierAppIDExclude ExtensionIdentifier = "appidExclude"
	// ExtensionIdentifierCredentialProperties is the Credential Properties extension.
	ExtensionIdentifierCredentialProperties ExtensionIdentifier = "credProps"
	// ExtensionIdentifierLargeBlob is the Large Blob extension.
	ExtensionIdentifierLargeBlob ExtensionIdentifier = "largeBlob"
	// ExtensionIdentifierPayment is the Payment extension.
	ExtensionIdentifierPayment ExtensionIdentifier = "payment"
)

const (
	// PublicKeyCredentialHintSecurityKey hint for security key.
	PublicKeyCredentialHintSecurityKey PublicKeyCredentialHint = "security-key"
	// PublicKeyCredentialHintClientDevice hint for client device.
	PublicKeyCredentialHintClientDevice PublicKeyCredentialHint = "client-device"
	// PublicKeyCredentialHintHybrid hint for hybrid.
	PublicKeyCredentialHintHybrid PublicKeyCredentialHint = "hybrid"
)

// PublicKeyCredentialRpEntity is used to supply additional Relying Party attributes when creating a new credential.
// https://www.w3.org/TR/webauthn-3/#dictdef-publickeycredentialrpentity
type PublicKeyCredentialRpEntity struct {
	ID   string `cbor:"id"`
	Name string `cbor:"name,omitempty"`
}

// PublicKeyCredentialUserEntity is used to supply additional user account attributes when creating a new credential.
// https://www.w3.org/TR/webauthn-3/#dictdef-publickeycredentialuserentity
type PublicKeyCredentialUserEntity struct {
	ID          []byte `cbor:"id"`
	DisplayName string `cbor:"displayName,omitempty"`
	Name        string `cbor:"name,omitempty"`
	Icon        string `cbor:"icon,omitempty"` // deprecated
}

// PublicKeyCredentialDescriptor identifies a specific public key credential.
// https://www.w3.org/TR/webauthn-3/#dictdef-publickeycredentialdescriptor
type PublicKeyCredentialDescriptor struct {
	Type       PublicKeyCredentialType  `cbor:"type"`
	ID         []byte                   `cbor:"id"`
	Transports []AuthenticatorTransport `cbor:"transports,omitempty"`
}

// PublicKeyCredentialParameters is used to supply additional parameters when creating a new credential.
// https://www.w3.org/TR/webauthn-3/#dictdef-publickeycredentialparameters
type PublicKeyCredentialParameters struct {
	Type      PublicKeyCredentialType `cbor:"type"`
	Algorithm key.Alg                 `cbor:"alg"`
}

// PackedAttestationStatementFormat is a WebAuthn optimized attestation statement format.
// https://www.w3.org/TR/webauthn-3/#sctn-packed-attestation
type PackedAttestationStatementFormat struct {
	Algorithm key.Alg  `cbor:"alg"`
	Signature []byte   `cbor:"sig"`
	X509Chain [][]byte `cbor:"x5c"`
}

// FIDOU2FAttestationStatementFormat is attestation statement format is used with FIDO U2F authenticators.
// https://www.w3.org/TR/webauthn-3/#sctn-fido-u2f-attestation
type FIDOU2FAttestationStatementFormat struct {
	X509Chain [][]byte `cbor:"x5c"`
	Signature []byte   `cbor:"sig"`
}

// TPMAttestationStatementFormat is generally used by authenticators that use a Trusted Platform Module
// as their cryptographic engine.
// https://www.w3.org/TR/webauthn-3/#sctn-tpm-attestation
type TPMAttestationStatementFormat struct {
	Version   string   `cbor:"ver"`
	Algorithm key.Alg  `cbor:"alg"`
	X509Chain [][]byte `cbor:"x5c"`
	AIKCert   []byte   `cbor:"aikCert"`
	Signature []byte   `cbor:"sig"`
	CertInfo  []byte   `cbor:"certInfo"` // TPMS_ATTEST structure
	PubArea   []byte   `cbor:"pubArea"`  // TPMT_PUBLIC structure
}
