package webauthn

// CreateMinPinLengthInputs represents the input for 'minPinLength' extension during creation.
type CreateMinPinLengthInputs struct {
	MinPinLength bool `cbor:"minPinLength"`
}
