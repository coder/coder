// Package fido2 provides shell-out wrappers for the coder-fido2
// helper binary that handles CTAP2 USB HID communication. The main
// coder binary stays pure Go; all CGo/libfido2 interaction is
// isolated in the helper.
package fido2

import (
	"bytes"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"

	"golang.org/x/xerrors"
)

const helperBinary = "coder-fido2"

// ErrTouchTimeout indicates the user did not touch the security key
// in time. Callers should prompt the user to try again.
var ErrTouchTimeout = xerrors.New("security key touch timed out")

// ErrPinRequired indicates the security key requires a PIN that was
// not provided or was incorrect.
var ErrPinRequired = xerrors.New("security key requires a PIN")

// runHelper executes the coder-fido2 helper with the given
// subcommand, piping inputJSON, PIN, and origin via stdin.
// Returns the stdout bytes on success.
//
// Stdin format (3 lines):
//
//	Line 1: JSON payload (creation or assertion options)
//	Line 2: PIN (empty for touch-only)
//	Line 3: Origin URL for clientDataJSON (e.g. "https://coder.example.com")
func runHelper(subcmd string, inputJSON []byte, pin string, origin string) ([]byte, error) {
	cmd := exec.Command(helperBinary, subcmd)

	var stdinBuf bytes.Buffer
	stdinBuf.Write(inputJSON)
	stdinBuf.WriteString("\n")
	stdinBuf.WriteString(pin)
	stdinBuf.WriteString("\n")
	stdinBuf.WriteString(origin)
	stdinBuf.WriteString("\n")
	cmd.Stdin = &stdinBuf

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			switch exitErr.ExitCode() {
			case 2:
				return nil, ErrTouchTimeout
			case 3:
				return nil, ErrPinRequired
			}
		}
		return nil, xerrors.Errorf("coder-fido2 %s: %s: %w",
			subcmd, strings.TrimSpace(errBuf.String()), err)
	}
	return outBuf.Bytes(), nil
}

// RunRegister shells out to the coder-fido2 helper to perform a
// WebAuthn registration (CTAP2 MakeCredential). The optionsJSON is
// the CredentialCreation options from the server's begin endpoint.
// Origin is the server's access URL (scheme://host[:port]).
// Returns the raw attestation response JSON to send to the server's
// finish endpoint.
func RunRegister(optionsJSON json.RawMessage, pin string, origin string) (json.RawMessage, error) {
	out, err := runHelper("register", optionsJSON, pin, origin)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(out), nil
}

// RunAssert shells out to the coder-fido2 helper to perform a
// WebAuthn assertion (CTAP2 GetAssertion). The optionsJSON is the
// CredentialAssertion options from the server's challenge endpoint.
// Origin is the server's access URL (scheme://host[:port]).
// Returns the raw assertion response JSON to send to the server's
// verify endpoint.
func RunAssert(optionsJSON json.RawMessage, pin string, origin string) (json.RawMessage, error) {
	out, err := runHelper("assert", optionsJSON, pin, origin)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(out), nil
}

// IsHelperInstalled returns true if the coder-fido2 helper binary
// is found on PATH.
func IsHelperInstalled() bool {
	_, err := exec.LookPath(helperBinary)
	return err == nil
}
