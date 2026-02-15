package webauthn

// LargeBlobSupport defines the level of support for large blobs.
type LargeBlobSupport string

const (
	// LargeBlobSupportRequired means the extension is required.
	LargeBlobSupportRequired LargeBlobSupport = "required"
	// LargeBlobSupportPreferred means the extension is preferred.
	LargeBlobSupportPreferred LargeBlobSupport = "preferred"
)

// AuthenticationExtensionsLargeBlobInputs represents the specific inputs for large blob extension.
type AuthenticationExtensionsLargeBlobInputs struct {
	Support LargeBlobSupport `cbor:"support"`
	Read    bool             `cbor:"read"`
	Write   []byte           `cbor:"write"`
}

// LargeBlobInputs wraps the large blob inputs.
type LargeBlobInputs struct {
	LargeBlob AuthenticationExtensionsLargeBlobInputs `cbor:"largeBlob"`
}

// AuthenticationExtensionsLargeBlobOutputs represents the specific outputs for large blob extension.
type AuthenticationExtensionsLargeBlobOutputs struct {
	Supported bool   `cbor:"supported"`
	Blob      []byte `cbor:"blob"`
	Written   bool   `cbor:"written"`
}

// LargeBlobOutputs wraps the large blob outputs.
type LargeBlobOutputs struct {
	LargeBlob AuthenticationExtensionsLargeBlobOutputs `cbor:"largeBlob"`
}
