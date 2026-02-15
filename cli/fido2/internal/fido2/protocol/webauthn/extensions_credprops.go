package webauthn

// CreateCredentialPropertiesInputs represents the input for 'credProps' extension during creation.
type CreateCredentialPropertiesInputs struct {
	CredentialProperties bool `cbor:"credProps"`
}

// CredentialPropertiesOutput contains the resident key property.
type CredentialPropertiesOutput struct {
	ResidentKey bool `cbor:"rk"`
}

// CreateCredentialPropertiesOutputs represents the output for 'credProps' extension after creation.
type CreateCredentialPropertiesOutputs struct {
	CredentialProperties CredentialPropertiesOutput `cbor:"credProps"`
}
