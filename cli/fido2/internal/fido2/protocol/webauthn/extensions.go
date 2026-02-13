package webauthn

// CreateAuthenticationExtensionsClientInputs represents the client extension inputs for credential creation.
type CreateAuthenticationExtensionsClientInputs struct {
	*CreateCredentialBlobInputs
	*CreateCredentialPropertiesInputs
	*CreateCredentialProtectionInputs
	*CreateHMACSecretInputs
	*CreateHMACSecretMCInputs
	*LargeBlobInputs
	*CreateMinPinLengthInputs
	*CreatePinComplexityPolicyInputs
	*PRFInputs
}

// CreateAuthenticationExtensionsClientOutputs represents the client extension outputs for credential creation.
type CreateAuthenticationExtensionsClientOutputs struct {
	*CreateCredentialBlobOutputs
	*CreateCredentialPropertiesOutputs
	*CreateHMACSecretOutputs
	*CreateHMACSecretMCOutputs
	*LargeBlobOutputs
	*PRFOutputs
}

// GetAuthenticationExtensionsClientInputs represents the client extension inputs for assertion retrieval.
type GetAuthenticationExtensionsClientInputs struct {
	*GetCredentialBlobInputs
	*GetHMACSecretInputs
	*LargeBlobInputs
	*PRFInputs
}

// GetAuthenticationExtensionsClientOutputs represents the client extension outputs for assertion retrieval.
type GetAuthenticationExtensionsClientOutputs struct {
	*GetCredentialBlobOutputs
	*GetHMACSecretOutputs
	*LargeBlobOutputs
	*PRFOutputs
}
