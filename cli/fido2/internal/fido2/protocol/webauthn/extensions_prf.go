package webauthn

// AuthenticationExtensionsPRFValues represents the salt values for PRF extension.
type AuthenticationExtensionsPRFValues struct {
	First  []byte `cbor:"first"`
	Second []byte `cbor:"second,omitempty"`
}

// AuthenticationExtensionsPRFInputs represents the inputs for PRF extension.
type AuthenticationExtensionsPRFInputs struct {
	Eval             *AuthenticationExtensionsPRFValues           `cbor:"eval,omitempty"`
	EvalByCredential map[string]AuthenticationExtensionsPRFValues `cbor:"evalByCredential,omitempty"`
}

// PRFInputs wraps the PRF extension inputs.
type PRFInputs struct {
	PRF AuthenticationExtensionsPRFInputs `cbor:"prf"`
}

// AuthenticationExtensionsPRFOutputs represents the outputs for PRF extension.
type AuthenticationExtensionsPRFOutputs struct {
	Enabled bool                              `cbor:"enabled"`
	Results AuthenticationExtensionsPRFValues `cbor:"results"`
}

// PRFOutputs wraps the PRF extension outputs.
type PRFOutputs struct {
	PRF AuthenticationExtensionsPRFOutputs `cbor:"prf"`
}
