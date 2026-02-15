package ctap2

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"iter"
	"slices"

	"github.com/fxamacker/cbor/v2"
	"github.com/coder/coder/v2/cli/fido2/internal/fido2/protocol/ctaphid"

	"github.com/ldclabs/cose/key"
	"github.com/coder/coder/v2/cli/fido2/internal/fido2/protocol/webauthn"
)

// CTAPHIDClient implements the Client interface using CTAPHID.
type CTAPHIDClient struct {
	ctaphidClient *ctaphid.Client
	cborEncMode   cbor.EncMode

	cid ctaphid.ChannelID
}

// NewClient creates a new CTAP2 client over CTAPHID.
// It initializes the communication by sending a CTAPHID_INIT command with a random nonce.
func NewClient(
	ctaphidClient *ctaphid.Client,
	cborEncMode cbor.EncMode,
) (*CTAPHIDClient, error) {
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	ctaphidInitResp, err := ctaphidClient.Init(ctaphid.BroadcastCID, nonce)
	if err != nil {
		return nil, err
	}

	return &CTAPHIDClient{
		ctaphidClient: ctaphidClient,
		cborEncMode:   cborEncMode,
		cid:           ctaphidInitResp.CID,
	}, nil
}

// Close closes the underlying CTAPHID connection.
func (c *CTAPHIDClient) Close() error {
	return c.ctaphidClient.Close()
}

// MakeCredential performs the AuthenticatorMakeCredential operation.
func (c *CTAPHIDClient) MakeCredential(
	pinUvAuthProtocolType PinUvAuthProtocolType,
	pinUvAuthToken []byte,
	clientData []byte,
	rp webauthn.PublicKeyCredentialRpEntity,
	user webauthn.PublicKeyCredentialUserEntity,
	pubKeyCredParams []webauthn.PublicKeyCredentialParameters,
	excludeList []webauthn.PublicKeyCredentialDescriptor,
	extensions *CreateExtensionInputs,
	options map[Option]bool,
	enterpriseAttestation uint,
	attestationFormatsPreference []webauthn.AttestationStatementFormatIdentifier,
) (*AuthenticatorMakeCredentialResponse, error) {
	hasher := sha256.New()
	hasher.Write(clientData)
	clientDataHash := hasher.Sum(nil)

	req := &AuthenticatorMakeCredentialRequest{
		ClientDataHash:               clientDataHash,
		RP:                           rp,
		User:                         user,
		PubKeyCredParams:             pubKeyCredParams,
		ExcludeList:                  excludeList,
		Extensions:                   extensions,
		Options:                      options,
		EnterpriseAttestation:        enterpriseAttestation,
		AttestationFormatsPreference: attestationFormatsPreference,
	}

	if pinUvAuthToken != nil {
		pinUvAuthParam, err := AuthenticateWithError(
			pinUvAuthProtocolType,
			pinUvAuthToken,
			clientDataHash,
		)
		if err != nil {
			return nil, err
		}

		req.PinUvAuthParam = pinUvAuthParam
		req.PinUvAuthProtocol = pinUvAuthProtocolType
	}

	b, err := c.cborEncMode.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("cannot marshal MakeCredential CBOR request: %w", err)
	}

	respRaw, err := c.ctaphidClient.CBOR(c.cid, slices.Concat([]byte{byte(CMDAuthenticatorMakeCredential)}, b))
	if err != nil {
		return nil, err
	}

	var resp *AuthenticatorMakeCredentialResponse
	if err := cbor.Unmarshal(respRaw.Data, &resp); err != nil {
		return nil, err
	}
	resp.AuthData, err = ParseMakeCredentialAuthData(resp.AuthDataRaw)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *CTAPHIDClient) GetAssertion(
	pinUvAuthProtocolType PinUvAuthProtocolType,
	pinUvAuthToken []byte,
	rpID string,
	clientData []byte,
	allowList []webauthn.PublicKeyCredentialDescriptor,
	extensions *GetExtensionInputs,
	options map[Option]bool,
) iter.Seq2[*AuthenticatorGetAssertionResponse, error] {
	return func(yield func(*AuthenticatorGetAssertionResponse, error) bool) {
		hasher := sha256.New()
		hasher.Write(clientData)
		clientDataHash := hasher.Sum(nil)

		req := &AuthenticatorGetAssertionRequest{
			RPID:           rpID,
			ClientDataHash: clientDataHash,
			AllowList:      allowList,
			Extensions:     extensions,
			Options:        options,
		}

		if pinUvAuthToken != nil {
			pinUvAuthParamBegin, err := AuthenticateWithError(
				pinUvAuthProtocolType,
				pinUvAuthToken,
				clientDataHash,
			)
			if err != nil {
				yield(nil, err)
				return
			}

			req.PinUvAuthParam = pinUvAuthParamBegin
			req.PinUvAuthProtocol = pinUvAuthProtocolType
		}

		bBegin, err := c.cborEncMode.Marshal(req)
		if err != nil {
			yield(nil, err)
			return
		}

		respRawBegin, err := c.ctaphidClient.CBOR(
			c.cid,
			slices.Concat([]byte{byte(CMDAuthenticatorGetAssertion)}, bBegin),
		)
		if err != nil {
			yield(nil, err)
			return
		}

		var respBegin *AuthenticatorGetAssertionResponse
		if err := cbor.Unmarshal(respRawBegin.Data, &respBegin); err != nil {
			yield(nil, err)
			return
		}
		respBegin.AuthData, err = ParseGetAssertionAuthData(respBegin.AuthDataRaw)
		if err != nil {
			yield(nil, err)
			return
		}

		if !yield(respBegin, nil) {
			return
		}

		for i := uint(1); i < respBegin.NumberOfCredentials; i++ {
			respRaw, err := c.ctaphidClient.CBOR(c.cid, []byte{byte(CMDAuthenticatorGetNextAssertion)})
			if err != nil {
				yield(nil, err)
				return
			}

			var resp *AuthenticatorGetAssertionResponse
			if err := cbor.Unmarshal(respRaw.Data, &resp); err != nil {
				yield(nil, err)
				return
			}
			resp.AuthData, err = ParseGetAssertionAuthData(resp.AuthDataRaw)
			if err != nil {
				yield(nil, err)
				return
			}

			if !yield(resp, nil) {
				return
			}
		}
	}
}

func (c *CTAPHIDClient) GetInfo() (*AuthenticatorGetInfoResponse, error) {
	respRaw, err := c.ctaphidClient.CBOR(c.cid, []byte{byte(CMDAuthenticatorGetInfo)})
	if err != nil {
		return nil, err
	}

	var resp *AuthenticatorGetInfoResponse
	if err := cbor.Unmarshal(respRaw.Data, &resp); err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *CTAPHIDClient) GetKeyAgreement(
	pinUvAuthProtocolType PinUvAuthProtocolType,
) (key.Key, error) {
	req := &AuthenticatorClientPINRequest{
		PinUvAuthProtocol: pinUvAuthProtocolType,
		SubCommand:        ClientPINSubCommandGetKeyAgreement,
	}

	b, err := c.cborEncMode.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("cannot marshal keyAgreement CBOR request: %w", err)
	}

	respRaw, err := c.ctaphidClient.CBOR(c.cid, slices.Concat([]byte{byte(CMDAuthenticatorClientPIN)}, b))
	if err != nil {
		return nil, fmt.Errorf("keyAgreement CBOR request failed: %w", err)
	}

	var resp *AuthenticatorClientPINResponse
	if err := cbor.Unmarshal(respRaw.Data, &resp); err != nil {
		return nil, fmt.Errorf("cannot unmarshal keyAgreement CBOR response: %w", err)
	}

	return resp.KeyAgreement, nil
}

// GetPinToken allows getting a PinUvAuthToken (superseded by GetPinUvAuthTokenUsingUvWithPermissions or
// GetPinUvAuthTokenUsingPinWithPermissions, thus for backwards compatibility only).
func (c *CTAPHIDClient) GetPinToken(
	pinUvAuthProtocolType PinUvAuthProtocolType,
	keyAgreement key.Key,
	pin string,
) ([]byte, error) {
	protocol, err := NewPinUvAuthProtocol(pinUvAuthProtocolType)
	if err != nil {
		return nil, err
	}

	platformCoseKey, sharedSecret, err := protocol.Encapsulate(keyAgreement)
	if err != nil {
		return nil, err
	}

	hasher := sha256.New()
	hasher.Write([]byte(pin))
	pinHash := hasher.Sum(nil)[:16]

	pinHashEnc, err := protocol.Encrypt(sharedSecret, pinHash)
	if err != nil {
		return nil, err
	}

	req := &AuthenticatorClientPINRequest{
		PinUvAuthProtocol: protocol.Type,
		SubCommand:        ClientPINSubCommandGetPinToken,
		KeyAgreement:      platformCoseKey,
		PinHashEnc:        pinHashEnc,
	}

	b, err := c.cborEncMode.Marshal(req)
	if err != nil {
		return nil, err
	}

	respRaw, err := c.ctaphidClient.CBOR(c.cid, slices.Concat([]byte{byte(CMDAuthenticatorClientPIN)}, b))
	if err != nil {
		return nil, err
	}

	var resp *AuthenticatorClientPINResponse
	if err := cbor.Unmarshal(respRaw.Data, &resp); err != nil {
		return nil, err
	}

	pinUvAuthToken, err := protocol.Decrypt(sharedSecret, resp.PinUvAuthToken)
	if err != nil {
		return nil, err
	}

	return pinUvAuthToken, nil
}

// GetPinUvAuthTokenUsingPinWithPermissions allows getting a PinUvAuthToken with specific permissions using PIN.
func (c *CTAPHIDClient) GetPinUvAuthTokenUsingPinWithPermissions(
	pinUvAuthProtocolType PinUvAuthProtocolType,
	keyAgreement key.Key,
	pin string,
	permissions Permission,
	rpID string,
) ([]byte, error) {
	protocol, err := NewPinUvAuthProtocol(pinUvAuthProtocolType)
	if err != nil {
		return nil, err
	}

	platformCoseKey, sharedSecret, err := protocol.Encapsulate(keyAgreement)
	if err != nil {
		return nil, err
	}

	hasher := sha256.New()
	hasher.Write([]byte(pin))
	pinHash := hasher.Sum(nil)[:16]

	pinHashEnc, err := protocol.Encrypt(sharedSecret, pinHash)
	if err != nil {
		return nil, err
	}

	req := &AuthenticatorClientPINRequest{
		PinUvAuthProtocol: protocol.Type,
		SubCommand:        ClientPINSubCommandGetPinUvAuthTokenUsingPinWithPermissions,
		KeyAgreement:      platformCoseKey,
		PinHashEnc:        pinHashEnc,
		Permissions:       permissions,
		RPID:              rpID,
	}

	b, err := c.cborEncMode.Marshal(req)
	if err != nil {
		return nil, err
	}

	respRaw, err := c.ctaphidClient.CBOR(c.cid, slices.Concat([]byte{byte(CMDAuthenticatorClientPIN)}, b))
	if err != nil {
		return nil, err
	}

	var resp *AuthenticatorClientPINResponse
	if err := cbor.Unmarshal(respRaw.Data, &resp); err != nil {
		return nil, err
	}

	pinUvAuthToken, err := protocol.Decrypt(sharedSecret, resp.PinUvAuthToken)
	if err != nil {
		return nil, err
	}

	return pinUvAuthToken, nil
}
