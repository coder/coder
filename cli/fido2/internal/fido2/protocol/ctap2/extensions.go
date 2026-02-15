package ctap2

import "github.com/ldclabs/cose/key"

// CreateCredProtectInput represents input for the 'credProtect' extension during creation.
type CreateCredProtectInput struct {
	CredProtect int `cbor:"credProtect"`
}

// CreateCredBlobInput represents input for the 'credBlob' extension during creation.
type CreateCredBlobInput struct {
	CredBlob []byte `cbor:"credBlob"`
}

// CreateMinPinLengthInput represents input for the 'minPinLength' extension during creation.
type CreateMinPinLengthInput struct {
	MinPinLength bool `cbor:"minPinLength"`
}

// CreatePinComplexityPolicyInput represents input for the 'pinComplexityPolicy' extension during creation.
type CreatePinComplexityPolicyInput struct {
	PinComplexityPolicy bool `cbor:"pinComplexityPolicy"`
}

// CreateHMACSecretInput represents input for the 'hmac-secret' extension during creation.
type CreateHMACSecretInput struct {
	HMACSecret bool `cbor:"hmac-secret"`
}

// CreateHMACSecretMCInput represents input for the 'hmac-secret-mc' extension during creation.
type CreateHMACSecretMCInput struct {
	HMACSecret HMACSecret `cbor:"hmac-secret-mc"`
}

// CreateThirdPartyPaymentInput represents input for the 'thirdPartyPayment' extension during creation.
type CreateThirdPartyPaymentInput struct {
	ThirdPartyPayment bool `cbor:"thirdPartyPayment"`
}

// CreateExtensionInputs aggregates all extension inputs for credential creation.
type CreateExtensionInputs struct {
	*CreateCredProtectInput
	*CreateCredBlobInput
	*CreateMinPinLengthInput
	*CreatePinComplexityPolicyInput
	*CreateHMACSecretInput
	*CreateHMACSecretMCInput
	*CreateThirdPartyPaymentInput
}

// CreateCredProtectOutput represents output for the 'credProtect' extension after creation.
type CreateCredProtectOutput struct {
	CredProtect int `cbor:"credProtect"`
}

// CreateCredBlobOutput represents output for the 'credBlob' extension after creation.
type CreateCredBlobOutput struct {
	CredBlob bool `cbor:"credBlob"`
}

// CreateMinPinLengthOutput represents output for the 'minPinLength' extension after creation.
type CreateMinPinLengthOutput struct {
	MinPinLength uint `cbor:"minPinLength"`
}

// CreatePinComplexityPolicyOutput represents output for the 'pinComplexityPolicy' extension after creation.
type CreatePinComplexityPolicyOutput struct {
	PinComplexityPolicy bool `cbor:"pinComplexityPolicy"`
}

// CreateHMACSecretOutput represents output for the 'hmac-secret' extension after creation.
type CreateHMACSecretOutput struct {
	HMACSecret bool `cbor:"hmac-secret"`
}

// CreateHMACSecretMCOutput represents output for the 'hmac-secret-mc' extension after creation.
type CreateHMACSecretMCOutput struct {
	HMACSecret []byte `cbor:"hmac-secret-mc"`
}

// CreateExtensionOutputs aggregates all extension outputs for credential creation.
type CreateExtensionOutputs struct {
	*CreateCredProtectOutput
	*CreateCredBlobOutput
	*CreateMinPinLengthOutput
	*CreatePinComplexityPolicyOutput
	*CreateHMACSecretOutput
	*CreateHMACSecretMCOutput
}

// GetCredBlobInput represents input for the 'credBlob' extension during assertion.
type GetCredBlobInput struct {
	CredBlob bool `cbor:"credBlob"`
}

// HMACSecret represents parameters for HMAC Secret extension.
type HMACSecret struct {
	KeyAgreement      key.Key               `cbor:"1,keyasint"`
	SaltEnc           []byte                `cbor:"2,keyasint"`
	SaltAuth          []byte                `cbor:"3,keyasint"`
	PinUvAuthProtocol PinUvAuthProtocolType `cbor:"4,keyasint,omitempty"`
}

// GetHMACSecretInput represents input for the 'hmac-secret' extension during assertion.
type GetHMACSecretInput struct {
	HMACSecret HMACSecret `cbor:"hmac-secret"`
}

// GetThirdPartyPaymentInput represents input for the 'thirdPartyPayment' extension during assertion.
type GetThirdPartyPaymentInput struct {
	ThirdPartyPayment bool `cbor:"thirdPartyPayment"`
}

// GetExtensionInputs aggregates all extension inputs for assertion.
type GetExtensionInputs struct {
	*GetCredBlobInput
	*GetHMACSecretInput
	*GetThirdPartyPaymentInput
}

// GetCredBlobOutput represents output for the 'credBlob' extension after assertion.
type GetCredBlobOutput struct {
	CredBlob []byte `cbor:"credBlob"`
}

// GetHMACSecretOutput represents output for the 'hmac-secret' extension after assertion.
type GetHMACSecretOutput struct {
	HMACSecret []byte `cbor:"hmac-secret"`
}

// GetThirdPartyPaymentOutput represents output for the 'thirdPartyPayment' extension after assertion.
type GetThirdPartyPaymentOutput struct {
	ThirdPartyPayment bool `cbor:"thirdPartyPayment"`
}

// GetExtensionOutputs aggregates all extension outputs for assertion.
type GetExtensionOutputs struct {
	*GetCredBlobOutput
	*GetHMACSecretOutput
	*GetThirdPartyPaymentOutput
}
