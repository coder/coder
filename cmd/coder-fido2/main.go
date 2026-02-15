// Command coder-fido2 is a helper binary for FIDO2 device interaction
// using the WebAuthn protocol. It is built separately with CGo +
// libfido2 so the main coder binary stays pure Go. This is a
// standalone Go module.
//
// Usage:
//
//	coder-fido2 register   (reads creation options JSON + PIN from stdin)
//	coder-fido2 assert     (reads assertion options JSON + PIN from stdin)
//
// Stdin format: first line is JSON payload, second line is PIN
// (empty for touch-only). Stdout is the response JSON.
//
// Exit codes: 0 = success, 1 = error, 2 = touch timeout, 3 = PIN required.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/fxamacker/cbor/v2"
	libfido2 "github.com/keys-pub/go-libfido2"
)

const (
	exitCodeTimeout     = 2
	exitCodePinRequired = 3
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "register":
		cmdRegister()
	case "assert":
		cmdAssert()
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: coder-fido2 <register|assert>\n")
	fmt.Fprintf(os.Stderr, "  Reads JSON options on first stdin line, PIN on second line.\n")
	os.Exit(1)
}

// readStdinLines reads three lines from stdin: JSON payload, PIN,
// and origin URL. The origin is the server's access URL used in
// clientDataJSON to match the server's RP origin configuration.
func readStdinLines() (jsonLine string, pin string, origin string) {
	scanner := bufio.NewScanner(os.Stdin)
	// Increase buffer for large JSON payloads.
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024)
	if scanner.Scan() {
		jsonLine = scanner.Text()
	}
	if scanner.Scan() {
		pin = strings.TrimRight(scanner.Text(), "\r\n")
	}
	if scanner.Scan() {
		origin = strings.TrimRight(scanner.Text(), "\r\n")
	}
	return
}

func discoverDevice() (*libfido2.Device, *libfido2.DeviceLocation) {
	locs, err := libfido2.DeviceLocations()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: enumerate devices: %v\n", err)
		os.Exit(1)
	}
	if len(locs) == 0 {
		fmt.Fprintf(os.Stderr, "error: no FIDO2 device found; plug in your security key\n")
		os.Exit(1)
	}
	device, err := libfido2.NewDevice(locs[0].Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: open device: %v\n", err)
		os.Exit(1)
	}
	return device, locs[0]
}

func isTouchTimeout(err error) bool {
	return errors.Is(err, libfido2.ErrOperationDenied) ||
		errors.Is(err, libfido2.ErrActionTimeout) ||
		strings.Contains(err.Error(), "operation denied") ||
		strings.Contains(err.Error(), "timed out")
}

func isPinRequired(err error) bool {
	return errors.Is(err, libfido2.ErrPinRequired) ||
		errors.Is(err, libfido2.ErrPinInvalid) ||
		strings.Contains(err.Error(), "pin required") ||
		strings.Contains(err.Error(), "pin invalid")
}

func exitForFIDOError(context string, err error) {
	if isTouchTimeout(err) {
		fmt.Fprintf(os.Stderr, "error: %s: touch timed out (try again)\n", context)
		os.Exit(exitCodeTimeout)
	}
	if isPinRequired(err) {
		fmt.Fprintf(os.Stderr, "error: %s: security key requires a PIN\n", context)
		os.Exit(exitCodePinRequired)
	}
	fmt.Fprintf(os.Stderr, "error: %s: %v\n", context, err)
	os.Exit(1)
}

// creationOptions is a minimal subset of the WebAuthn
// CredentialCreation options that we need for CTAP2.
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

// attestationResponse is the response format expected by the
// go-webauthn server library's ParseCredentialCreationResponseBody.
type attestationResponse struct {
	ID       string `json:"id"`
	RawID    string `json:"rawId"`
	Type     string `json:"type"`
	Response struct {
		AttestationObject string `json:"attestationObject"`
		ClientDataJSON    string `json:"clientDataJSON"`
	} `json:"response"`
}

func cmdRegister() {
	jsonLine, pin, origin := readStdinLines()
	device, loc := discoverDevice()

	fmt.Fprintf(os.Stderr, "Found: %s %s at %s\n", loc.Manufacturer, loc.Product, loc.Path)

	var opts creationOptions
	if err := json.Unmarshal([]byte(jsonLine), &opts); err != nil {
		fmt.Fprintf(os.Stderr, "error: parse creation options: %v\n", err)
		os.Exit(1)
	}

	userID, err := base64Decode(opts.PublicKey.User.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: decode user ID: %v\n", err)
		os.Exit(1)
	}

	// Build clientDataJSON first — the CTAP2 device needs its
	// SHA-256 hash as the clientDataHash parameter.
	clientData := buildClientDataJSON("webauthn.create", opts.PublicKey.Challenge, origin)
	cdh := sha256.Sum256(clientData)

	// Find the first algorithm from the server's list that we
	// support. The server orders by preference.
	alg := libfido2.ES256
	algFound := false
	for _, p := range opts.PublicKey.PubKeyCredParams {
		switch p.Alg {
		case -7:
			alg = libfido2.ES256
			algFound = true
		case -257:
			alg = libfido2.RS256
			algFound = true
		}
		if algFound {
			break
		}
	}
	if !algFound && len(opts.PublicKey.PubKeyCredParams) > 0 {
		fmt.Fprintf(os.Stderr, "warning: no supported algorithm in server's list, defaulting to ES256\n")
	}

	fmt.Fprintf(os.Stderr, "Touch your security key to register...\n")

	attest, err := device.MakeCredential(
		cdh[:],
		libfido2.RelyingParty{
			ID:   opts.PublicKey.RP.ID,
			Name: opts.PublicKey.RP.Name,
		},
		libfido2.User{
			ID:          userID,
			Name:        opts.PublicKey.User.Name,
			DisplayName: opts.PublicKey.User.DisplayName,
		},
		alg,
		pin,
		&libfido2.MakeCredentialOpts{
			RK: libfido2.Default,
		},
	)
	if err != nil {
		exitForFIDOError("make credential", err)
	}

	// clientData was already built above for the clientDataHash.

	// The CTAP2 library returns raw authData. The WebAuthn protocol
	// expects an attestationObject which is CBOR-encoded and wraps
	// the authData with format and statement.
	attestObj, err := buildAttestationObject(attest.AuthData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: build attestation object: %v\n", err)
		os.Exit(1)
	}

	resp := attestationResponse{
		ID:    base64Encode(attest.CredentialID),
		RawID: base64Encode(attest.CredentialID),
		Type:  "public-key",
	}
	resp.Response.AttestationObject = base64Encode(attestObj)
	resp.Response.ClientDataJSON = base64Encode(clientData)

	out, err := json.Marshal(resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: marshal response: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(string(out))
}

// assertionOptions is a minimal subset of the WebAuthn
// CredentialAssertion options for CTAP2.
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

// assertionResponse is the response format expected by the
// go-webauthn server library's ParseCredentialRequestResponseBody.
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

func cmdAssert() {
	jsonLine, pin, origin := readStdinLines()
	device, loc := discoverDevice()

	fmt.Fprintf(os.Stderr, "Found: %s %s at %s\n", loc.Manufacturer, loc.Product, loc.Path)

	var opts assertionOptions
	if err := json.Unmarshal([]byte(jsonLine), &opts); err != nil {
		fmt.Fprintf(os.Stderr, "error: parse assertion options: %v\n", err)
		os.Exit(1)
	}

	// Build clientDataJSON first — the CTAP2 device needs its
	// SHA-256 hash as the clientDataHash parameter.
	clientData := buildClientDataJSON("webauthn.get", opts.PublicKey.Challenge, origin)
	cdh := sha256.Sum256(clientData)

	var credIDs [][]byte
	for _, ac := range opts.PublicKey.AllowCredentials {
		id, err := base64Decode(ac.ID)
		if err != nil {
			continue
		}
		credIDs = append(credIDs, id)
	}

	fmt.Fprintf(os.Stderr, "Touch your security key to authenticate...\n")

	assertion, err := device.Assertion(
		opts.PublicKey.RPID,
		cdh[:],
		credIDs,
		pin,
		&libfido2.AssertionOpts{
			UP: libfido2.True,
		},
	)
	if err != nil {
		exitForFIDOError("assertion", err)
	}

	// clientData was already built above for the clientDataHash.

	var usedCredID []byte
	if len(assertion.CredentialID) > 0 {
		usedCredID = assertion.CredentialID
	} else if len(credIDs) == 1 {
		usedCredID = credIDs[0]
	}

	resp := assertionResponse{
		ID:    base64Encode(usedCredID),
		RawID: base64Encode(usedCredID),
		Type:  "public-key",
	}
	// AuthDataCBOR from go-libfido2 is CBOR-encoded (byte string
	// wrapped). The WebAuthn protocol expects raw bytes, so we
	// CBOR-decode to get the inner bytes.
	rawAuthData, err := cborDecodeBytes(assertion.AuthDataCBOR)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: decode authData CBOR: %v\n", err)
		os.Exit(1)
	}
	resp.Response.AuthenticatorData = base64Encode(rawAuthData)
	resp.Response.ClientDataJSON = base64Encode(clientData)
	resp.Response.Signature = base64Encode(assertion.Sig)
	if len(assertion.User.ID) > 0 {
		resp.Response.UserHandle = base64Encode(assertion.User.ID)
	}

	out, err := json.Marshal(resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: marshal response: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(string(out))
}

// buildAttestationObject wraps authenticator data from CTAP2 into a
// CBOR-encoded WebAuthn attestation object with "none" format. The
// authDataCBOR from go-libfido2 is already CBOR-encoded (byte string
// wrapped), so we use cbor.RawMessage to prevent double-encoding.
func buildAttestationObject(authDataCBOR []byte) ([]byte, error) {
	obj := struct {
		Fmt      string          `cbor:"fmt"`
		AttStmt  map[string]any  `cbor:"attStmt"`
		AuthData cbor.RawMessage `cbor:"authData"`
	}{
		Fmt:      "none",
		AttStmt:  map[string]any{},
		AuthData: cbor.RawMessage(authDataCBOR),
	}
	return cbor.Marshal(obj)
}

// buildClientDataJSON constructs the clientDataJSON that a browser
// would normally generate. Since we're a CLI, we build it manually.
// The origin must match the server's RPOrigins exactly.
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

// cborDecodeBytes unwraps a CBOR-encoded byte string. The go-libfido2
// library returns AuthData and AuthDataCBOR as CBOR byte strings; we
// need the raw inner bytes for WebAuthn JSON responses.
func cborDecodeBytes(data []byte) ([]byte, error) {
	var raw []byte
	if err := cbor.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// base64Decode handles both standard and URL-safe base64.
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
