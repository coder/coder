package ctap2

import (
	"bytes"

	"github.com/fxamacker/cbor/v2"
	"github.com/ldclabs/cose/key"
	"github.com/coder/coder/v2/cli/fido2/internal/fido2/protocol/webauthn"
)

// AuthenticatorMakeCredentialRequest represents the request for AuthenticatorMakeCredential command.
type AuthenticatorMakeCredentialRequest struct {
	ClientDataHash               []byte                                          `cbor:"1,keyasint"`
	RP                           webauthn.PublicKeyCredentialRpEntity            `cbor:"2,keyasint"`
	User                         webauthn.PublicKeyCredentialUserEntity          `cbor:"3,keyasint"`
	PubKeyCredParams             []webauthn.PublicKeyCredentialParameters        `cbor:"4,keyasint"`
	ExcludeList                  []webauthn.PublicKeyCredentialDescriptor        `cbor:"5,keyasint,omitempty"`
	Extensions                   *CreateExtensionInputs                          `cbor:"6,keyasint,omitempty"`
	Options                      map[Option]bool                                 `cbor:"7,keyasint,omitempty"`
	PinUvAuthParam               []byte                                          `cbor:"8,keyasint,omitempty"`
	PinUvAuthProtocol            PinUvAuthProtocolType                           `cbor:"9,keyasint,omitempty"`
	EnterpriseAttestation        uint                                            `cbor:"10,keyasint,omitempty"`
	AttestationFormatsPreference []webauthn.AttestationStatementFormatIdentifier `cbor:"11,keyasint,omitempty"`
}

// AuthenticatorMakeCredentialResponse represents the response for AuthenticatorMakeCredential command.
type AuthenticatorMakeCredentialResponse struct {
	Format                   webauthn.AttestationStatementFormatIdentifier         `cbor:"1,keyasint"`
	AuthDataRaw              []byte                                                `cbor:"2,keyasint"`
	AuthData                 *MakeCredentialAuthData                               `cbor:"-"`
	AttestationStatement     map[string]any                                        `cbor:"3,keyasint,omitempty"`
	EnterpriseAttestation    bool                                                  `cbor:"4,keyasint,omitempty"`
	LargeBlobKey             []byte                                                `cbor:"5,keyasint,omitempty"`
	UnsignedExtensionOutputs map[webauthn.ExtensionIdentifier]any                  `cbor:"6,keyasint,omitempty"`
	ExtensionOutputs         *webauthn.CreateAuthenticationExtensionsClientOutputs `cbor:"-"`
}

// MakeCredentialAuthData represents the authenticator data returned in MakeCredential response.
type MakeCredentialAuthData struct {
	RPIDHash               []byte
	Flags                  AuthDataFlag
	SignCount              uint32
	AttestedCredentialData *AttestedCredentialData
	Extensions             *CreateExtensionOutputs
}

// ParseMakeCredentialAuthData parses the authenticator data for MakeCredential.
func ParseMakeCredentialAuthData(data []byte) (*MakeCredentialAuthData, error) {
	d, err := parseAuthData(data)
	if err != nil {
		return nil, err
	}

	makeCredentialAuthData := &MakeCredentialAuthData{
		RPIDHash:               d.RPIDHash,
		Flags:                  d.Flags,
		SignCount:              d.SignCount,
		AttestedCredentialData: d.AttestedCredentialData,
	}

	if d.Extensions != nil {
		if err := cbor.NewDecoder(bytes.NewReader(d.Extensions)).
			Decode(&makeCredentialAuthData.Extensions); err != nil {
			return nil, err
		}
	}

	return makeCredentialAuthData, nil
}

func (r *AuthenticatorMakeCredentialResponse) PackedAttestationStatementFormat() (*webauthn.PackedAttestationStatementFormat, bool) {
	algRaw, ok := r.AttestationStatement["alg"]
	if !ok {
		return nil, false
	}
	alg, ok := algRaw.(int64)
	if !ok {
		return nil, false
	}

	sigRaw, ok := r.AttestationStatement["sig"]
	if !ok {
		return nil, false
	}
	sig, ok := sigRaw.([]byte)
	if !ok {
		return nil, false
	}

	x5cRaw, ok := r.AttestationStatement["x5c"]
	if !ok {
		return nil, false
	}
	x5cSlice, ok := x5cRaw.([]any)
	if !ok {
		return nil, false
	}
	var x5c [][]byte
	for _, certRaw := range x5cSlice {
		cert, ok := certRaw.([]byte)
		if !ok {
			return nil, false
		}
		x5c = append(x5c, cert)
	}

	return &webauthn.PackedAttestationStatementFormat{
		Algorithm: key.Alg(alg),
		Signature: sig,
		X509Chain: x5c,
	}, true
}

func (r *AuthenticatorMakeCredentialResponse) FIDOU2FAttestationStatementFormat() (*webauthn.FIDOU2FAttestationStatementFormat, bool) {
	x5cRaw, ok := r.AttestationStatement["x5c"]
	if !ok {
		return nil, false
	}
	x5c, ok := x5cRaw.([][]byte)
	if !ok {
		return nil, false
	}

	sigRaw, ok := r.AttestationStatement["sig"]
	if !ok {
		return nil, false
	}
	sig, ok := sigRaw.([]byte)
	if !ok {
		return nil, false
	}

	return &webauthn.FIDOU2FAttestationStatementFormat{
		Signature: sig,
		X509Chain: x5c,
	}, true
}

func (r *AuthenticatorMakeCredentialResponse) TPMAttestationStatementFormat() (*webauthn.TPMAttestationStatementFormat, bool) {
	verRaw, ok := r.AttestationStatement["ver"]
	if !ok {
		return nil, false
	}
	ver, ok := verRaw.(string)
	if !ok {
		return nil, false
	}

	algRaw, ok := r.AttestationStatement["alg"]
	if !ok {
		return nil, false
	}
	alg, ok := algRaw.(int64)
	if !ok {
		return nil, false
	}

	x5cRaw, ok := r.AttestationStatement["x5c"]
	if !ok {
		return nil, false
	}
	x5cSlice, ok := x5cRaw.([]any)
	if !ok {
		return nil, false
	}
	var x5c [][]byte
	for _, certRaw := range x5cSlice {
		cert, ok := certRaw.([]byte)
		if !ok {
			return nil, false
		}
		x5c = append(x5c, cert)
	}

	aikCertRaw, ok := r.AttestationStatement["aikCert"]
	if !ok {
		return nil, false
	}
	aikCert, ok := aikCertRaw.([]byte)
	if !ok {
		return nil, false
	}

	sigRaw, ok := r.AttestationStatement["sig"]
	if !ok {
		return nil, false
	}
	sig, ok := sigRaw.([]byte)
	if !ok {
		return nil, false
	}

	certInfoRaw, ok := r.AttestationStatement["certInfo"]
	if !ok {
		return nil, false
	}
	certInfo, ok := certInfoRaw.([]byte)
	if !ok {
		return nil, false
	}

	pubAreaRaw, ok := r.AttestationStatement["pubArea"]
	if !ok {
		return nil, false
	}
	pubArea, ok := pubAreaRaw.([]byte)
	if !ok {
		return nil, false
	}

	return &webauthn.TPMAttestationStatementFormat{
		Version:   ver,
		Algorithm: key.Alg(alg),
		X509Chain: x5c,
		AIKCert:   aikCert,
		Signature: sig,
		CertInfo:  certInfo,
		PubArea:   pubArea,
	}, true
}
