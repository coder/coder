package webauthn

// CreateHMACSecretInputs represents the input for 'hmac-secret' extension during creation.
type CreateHMACSecretInputs struct {
	HMACCreateSecret bool `cbor:"hmacCreateSecret"`
}

// CreateHMACSecretOutputs represents the output for 'hmac-secret' extension after creation.
type CreateHMACSecretOutputs struct {
	HMACCreateSecret bool `cbor:"hmacCreateSecret"`
}

// HMACGetSecretInput represents the input salt(s) for HMAC secret retrieval.
type HMACGetSecretInput struct {
	Salt1 []byte `cbor:"salt1"`
	Salt2 []byte `cbor:"salt2,omitempty"`
}

// GetHMACSecretInputs represents the input for 'hmac-secret' extension during assertion.
type GetHMACSecretInputs struct {
	HMACGetSecret HMACGetSecretInput `cbor:"hmacGetSecret"`
}

// HMACGetSecretOutput represents the output salt(s) for HMAC secret retrieval.
type HMACGetSecretOutput struct {
	Output1 []byte `cbor:"output1"`
	Output2 []byte `cbor:"output2,omitempty"`
}

// GetHMACSecretOutputs represents the output for 'hmac-secret' extension after assertion.
type GetHMACSecretOutputs struct {
	HMACGetSecret HMACGetSecretOutput `cbor:"hmacGetSecret"`
}

// CreateHMACSecretMCInputs represents the input for 'hmac-secret-mc' extension during creation.
type CreateHMACSecretMCInputs struct {
	HMACGetSecret HMACGetSecretInput `cbor:"hmacGetSecret"`
}

// CreateHMACSecretMCOutputs represents the output for 'hmac-secret-mc' extension after creation.
type CreateHMACSecretMCOutputs struct {
	HMACGetSecret HMACGetSecretOutput `cbor:"hmacGetSecret"`
}
