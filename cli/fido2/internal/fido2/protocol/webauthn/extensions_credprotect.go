package webauthn

// CredentialProtectionPolicy defines the possible protection policies for a credential.
type CredentialProtectionPolicy string

const (
	// CredentialProtectionPolicyUserVerificationOptional policy means UV is optional.
	CredentialProtectionPolicyUserVerificationOptional CredentialProtectionPolicy = "userVerificationOptional"
	// CredentialProtectionPolicyUserVerificationOptionalWithCredentialIDList policy means UV is optional but recommended.
	CredentialProtectionPolicyUserVerificationOptionalWithCredentialIDList CredentialProtectionPolicy = "userVerificationOptionalWithCredentialIDList"
	// CredentialProtectionPolicyUserVerificationRequired policy means UV is required.
	CredentialProtectionPolicyUserVerificationRequired CredentialProtectionPolicy = "userVerificationRequired"
)

// CreateCredentialProtectionInputs represents the input for 'credProtect' extension during creation.
type CreateCredentialProtectionInputs struct {
	CredentialProtectionPolicy        CredentialProtectionPolicy `cbor:"credentialProtectionPolicy"`
	EnforceCredentialProtectionPolicy bool                       `cbor:"enforceCredentialProtectionPolicy"`
}
