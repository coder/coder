package webauthn

// CreateCredentialBlobInputs represents the input for 'credBlob' extension during creation.
type CreateCredentialBlobInputs struct {
	CredBlob []byte `cbor:"credBlob"`
}

// CreateCredentialBlobOutputs represents the output for 'credBlob' extension after creation.
type CreateCredentialBlobOutputs struct {
	CredBlob bool `cbor:"credBlob"`
}

// GetCredentialBlobInputs represents the input for 'credBlob' extension during assertion.
type GetCredentialBlobInputs struct {
	GetCredBlob bool `cbor:"getCredBlob"`
}

// GetCredentialBlobOutputs represents the output for 'credBlob' extension after assertion.
type GetCredentialBlobOutputs struct {
	GetCredBlob []byte `cbor:"getCredBlob"`
}
