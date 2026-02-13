// Package fido2 provides direct FIDO2 USB HID interaction using a
// pure Go library. No CGo or external helper binary is needed.
package fido2

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/fxamacker/cbor/v2"
	"github.com/ldclabs/cose/key"
	"golang.org/x/xerrors"

	gofido2 "github.com/coder/coder/v2/cli/fido2/internal/fido2"
	"github.com/coder/coder/v2/cli/fido2/internal/fido2/protocol/ctap2"
	"github.com/coder/coder/v2/cli/fido2/internal/fido2/protocol/webauthn"
)

// ErrTouchTimeout indicates the user did not touch the security key
// in time. Callers should prompt the user to try again.
var ErrTouchTimeout = xerrors.New("security key touch timed out")

// ErrPinRequired indicates the security key requires a PIN that was
// not provided or was incorrect.
var ErrPinRequired = xerrors.New("security key requires a PIN")

// ErrNoDevice indicates no FIDO2 security key was found.
var ErrNoDevice = xerrors.New("no FIDO2 security key found")

// creationOptions is the subset of WebAuthn CredentialCreation
// options we need to call CTAP2 MakeCredential.
type creationOptions struct {
	PublicKey struct {
		RP struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"rp"`
		User struct {
			ID          string `json:"id"` // base64url
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"user"`
		Challenge        string `json:"challenge"` // base64url
		PubKeyCredParams []struct {
			Type string `json:"type"`
			Alg  int    `json:"alg"`
		} `json:"pubKeyCredParams"`
	} `json:"publicKey"`
}

// assertionOptions is the subset of WebAuthn CredentialAssertion
// options we need to call CTAP2 GetAssertion.
type assertionOptions struct {
	PublicKey struct {
		Challenge        string `json:"challenge"` // base64url
		RPID             string `json:"rpId"`
		AllowCredentials []struct {
			ID   string `json:"id"` // base64url
			Type string `json:"type"`
		} `json:"allowCredentials"`
	} `json:"publicKey"`
}

// attestationResponse is the JSON format expected by the go-webauthn
// server library's ParseCredentialCreationResponseBody.
type attestationResponse struct {
	ID       string `json:"id"`
	RawID    string `json:"rawId"`
	Type     string `json:"type"`
	Response struct {
		AttestationObject string `json:"attestationObject"`
		ClientDataJSON    string `json:"clientDataJSON"`
	} `json:"response"`
}

// assertionResponse is the JSON format expected by the go-webauthn
// server library's ParseCredentialRequestResponseBody.
type assertionResponse struct {
	ID       string `json:"id"`
	RawID    string `json:"rawId"`
	Type     string `json:"type"`
	Response struct {
		AuthenticatorData string `json:"authenticatorData"`
		ClientDataJSON    string `json:"clientDataJSON"`
		Signature         string `json:"signature"`
		UserHandle        string `json:"userHandle,omitempty"`
	} `json:"response"`
}

// openDevice enumerates FIDO2 USB devices and opens the first one.
func openDevice() (*gofido2.Device, error) {
	devs, err := gofido2.Enumerate()
	if err != nil {
		return nil, xerrors.Errorf("enumerate FIDO2 devices: %w", err)
	}
	if len(devs) == 0 {
		return nil, ErrNoDevice
	}
	dev, err := gofido2.Open(devs[0])
	if err != nil {
		return nil, xerrors.Errorf("open FIDO2 device: %w", err)
	}
	return dev, nil
}

// classifyError maps CTAP2 errors to our sentinel errors.
// It checks both the vendored library's typed errors and
// string patterns for CTAP2 status codes.
func classifyError(err error) error {
	if err == nil {
		return nil
	}

	// Check vendored library sentinel errors first.
	if xerrors.Is(err, gofido2.ErrPinUvAuthTokenRequired) {
		return ErrPinRequired
	}

	msg := err.Error()
	if strings.Contains(msg, "timed out") ||
		strings.Contains(msg, "operation denied") ||
		strings.Contains(msg, "OPERATION_DENIED") ||
		strings.Contains(msg, "KEEPALIVE_CANCEL") ||
		strings.Contains(msg, "ACTION_TIMEOUT") ||
		strings.Contains(msg, "USER_ACTION_TIMEOUT") {
		return ErrTouchTimeout
	}
	if strings.Contains(msg, "pin required") ||
		strings.Contains(msg, "PIN_REQUIRED") ||
		strings.Contains(msg, "PIN_INVALID") ||
		strings.Contains(msg, "pinUvAuthToken required") {
		return ErrPinRequired
	}
	return err
}

// RunRegister performs a WebAuthn registration (CTAP2
// MakeCredential) by talking directly to the USB device.
func RunRegister(optionsJSON json.RawMessage, pin string, origin string) (json.RawMessage, error) {
	var opts creationOptions
	if err := json.Unmarshal(optionsJSON, &opts); err != nil {
		return nil, xerrors.Errorf("parse creation options: %w", err)
	}

	dev, err := openDevice()
	if err != nil {
		return nil, err
	}
	defer dev.Close()

	userID, err := base64Decode(opts.PublicKey.User.ID)
	if err != nil {
		return nil, xerrors.Errorf("decode user ID: %w", err)
	}

	// Build clientDataJSON — the go-fido2 library hashes it
	// internally to produce the clientDataHash for CTAP2.
	clientData := buildClientDataJSON("webauthn.create", opts.PublicKey.Challenge, origin)

	// Build pubKeyCredParams from the server's list.
	var params []webauthn.PublicKeyCredentialParameters
	for _, p := range opts.PublicKey.PubKeyCredParams {
		params = append(params, webauthn.PublicKeyCredentialParameters{
			Type:      webauthn.PublicKeyCredentialTypePublicKey,
			Algorithm: key.Alg(p.Alg),
		})
	}

	// If the caller provided a PIN, obtain a pinUvAuthToken
	// from the device. This is required when the server sets
	// UserVerification=required or the device enforces PIN.
	var pinUvAuthToken []byte
	if pin != "" {
		pinUvAuthToken, err = dev.GetPinUvAuthTokenUsingPIN(
			pin,
			ctap2.PermissionMakeCredential,
			opts.PublicKey.RP.ID,
		)
		if err != nil {
			return nil, classifyError(err)
		}
	}

	ctapOptions := map[ctap2.Option]bool{
		ctap2.OptionUserPresence: true,
	}
	if pin != "" {
		// When PIN is provided, request user verification so
		// the authenticator sets the UV flag in the response.
		ctapOptions[ctap2.OptionUserVerification] = true
	}

	resp, err := dev.MakeCredential(
		pinUvAuthToken,
		clientData,
		webauthn.PublicKeyCredentialRpEntity{
			ID:   opts.PublicKey.RP.ID,
			Name: opts.PublicKey.RP.Name,
		},
		webauthn.PublicKeyCredentialUserEntity{
			ID:          userID,
			Name:        opts.PublicKey.User.Name,
			DisplayName: opts.PublicKey.User.DisplayName,
		},
		params,
		nil, // no exclude list
		&webauthn.CreateAuthenticationExtensionsClientInputs{}, // empty
		ctapOptions,
		0,   // no enterprise attestation
		nil, // no attestation format preference
	)
	if err != nil {
		return nil, classifyError(err)
	}

	// Build the attestation object (CBOR with fmt, attStmt,
	// authData) from the CTAP2 response.
	attestObj, err := buildAttestationObject(resp)
	if err != nil {
		return nil, xerrors.Errorf("build attestation object: %w", err)
	}

	credID := resp.AuthData.AttestedCredentialData.CredentialID

	out := attestationResponse{
		ID:    base64Encode(credID),
		RawID: base64Encode(credID),
		Type:  "public-key",
	}
	out.Response.AttestationObject = base64Encode(attestObj)
	out.Response.ClientDataJSON = base64Encode(clientData)

	return json.Marshal(out)
}

// RunAssert performs a WebAuthn assertion (CTAP2 GetAssertion)
// by talking directly to the USB device.
func RunAssert(optionsJSON json.RawMessage, pin string, origin string) (json.RawMessage, error) {
	var opts assertionOptions
	if err := json.Unmarshal(optionsJSON, &opts); err != nil {
		return nil, xerrors.Errorf("parse assertion options: %w", err)
	}

	dev, err := openDevice()
	if err != nil {
		return nil, err
	}
	defer dev.Close()

	clientData := buildClientDataJSON("webauthn.get", opts.PublicKey.Challenge, origin)

	var allowList []webauthn.PublicKeyCredentialDescriptor
	for _, ac := range opts.PublicKey.AllowCredentials {
		id, decErr := base64Decode(ac.ID)
		if decErr != nil {
			continue
		}
		allowList = append(allowList, webauthn.PublicKeyCredentialDescriptor{
			Type: webauthn.PublicKeyCredentialTypePublicKey,
			ID:   id,
		})
	}

	// If the caller provided a PIN, obtain a pinUvAuthToken
	// from the device before the assertion. The token proves
	// user verification to the authenticator.
	var pinUvAuthToken []byte
	if pin != "" {
		pinUvAuthToken, err = dev.GetPinUvAuthTokenUsingPIN(
			pin,
			ctap2.PermissionGetAssertion,
			opts.PublicKey.RPID,
		)
		if err != nil {
			return nil, classifyError(err)
		}
	}

	ctapOptions := map[ctap2.Option]bool{
		ctap2.OptionUserPresence: true,
	}
	if pin != "" {
		ctapOptions[ctap2.OptionUserVerification] = true
	}

	// GetAssertion returns an iterator; we take the first result.
	for resp, iterErr := range dev.GetAssertion(
		pinUvAuthToken,
		opts.PublicKey.RPID,
		clientData,
		allowList,
		&webauthn.GetAuthenticationExtensionsClientInputs{}, // empty
		ctapOptions,
	) {
		if iterErr != nil {
			return nil, classifyError(iterErr)
		}

		credID := resp.Credential.ID

		out := assertionResponse{
			ID:    base64Encode(credID),
			RawID: base64Encode(credID),
			Type:  "public-key",
		}
		out.Response.AuthenticatorData = base64Encode(resp.AuthDataRaw)
		out.Response.ClientDataJSON = base64Encode(clientData)
		out.Response.Signature = base64Encode(resp.Signature)
		if resp.User != nil && len(resp.User.ID) > 0 {
			out.Response.UserHandle = base64Encode(resp.User.ID)
		}

		return json.Marshal(out)
	}

	return nil, xerrors.New("no assertion response from device")
}

// IsAvailable returns true if at least one FIDO2 USB device is
// detected. This replaces the old IsHelperInstalled check.
func IsAvailable() bool {
	devs, err := gofido2.Enumerate()
	return err == nil && len(devs) > 0
}

// buildAttestationObject creates the CBOR attestation object from
// a CTAP2 MakeCredential response. The authData is already in the
// correct binary format from the CTAP2 response.
func buildAttestationObject(resp *ctap2.AuthenticatorMakeCredentialResponse) ([]byte, error) {
	obj := struct {
		Fmt      string         `cbor:"fmt"`
		AttStmt  map[string]any `cbor:"attStmt"`
		AuthData []byte         `cbor:"authData"`
	}{
		Fmt:      string(resp.Format),
		AttStmt:  resp.AttestationStatement,
		AuthData: resp.AuthDataRaw,
	}
	if obj.AttStmt == nil {
		obj.AttStmt = map[string]any{}
	}
	return cbor.Marshal(obj)
}

func buildClientDataJSON(typ, challenge, origin string) []byte {
	cd := struct {
		Type      string `json:"type"`
		Challenge string `json:"challenge"`
		Origin    string `json:"origin"`
	}{
		Type:      typ,
		Challenge: challenge,
		Origin:    origin,
	}
	data, _ := json.Marshal(cd)
	return data
}

func base64Decode(s string) ([]byte, error) {
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		b, err = base64.RawURLEncoding.DecodeString(s)
	}
	return b, err
}

func base64Encode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}
