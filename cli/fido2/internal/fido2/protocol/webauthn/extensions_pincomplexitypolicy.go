package webauthn

// CreatePinComplexityPolicyInputs represents the input for 'pinComplexityPolicy' extension during creation.
type CreatePinComplexityPolicyInputs struct {
	PinComplexityPolicy bool `cbor:"pinComplexityPolicy"`
}
