package cli

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/coder/v2/cli/touchid"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

// SetupConnectAuth generates a Secure Enclave keypair and uploads
// the public key to the server. The encrypted key reference
// (dataRepresentation) is stored in the config directory. Called
// after login on macOS when Secure Enclave is available. keyID is
// the first 10 chars of the session token (before the '-').
func SetupConnectAuth(inv *serpent.Invocation, client *codersdk.Client, keyID string, cfg config.Root) error {
	// Check if Secure Enclave + biometrics are available.
	available, _, _, err := touchid.Diagnostic()
	if err != nil || !available {
		return nil
	}

	pubKeyB64, dataRepB64, err := touchid.GenerateKey()
	if err != nil {
		return xerrors.Errorf("generate Secure Enclave key: %w", err)
	}

	// Store the dataRepresentation to the config directory.
	// This is an encrypted blob that only the Secure Enclave
	// on this device can use — not the actual private key.
	err = cfg.ConnectKey().Write(dataRepB64)
	if err != nil {
		return xerrors.Errorf("write connect key data: %w", err)
	}

	err = client.UpdateAPIKeyConnectPublicKey(inv.Context(), codersdk.Me, keyID,
		codersdk.UpdateConnectPublicKeyRequest{
			PublicKey: pubKeyB64,
		})
	if err != nil {
		// Clean up on failure.
		_ = cfg.ConnectKey().Delete()
		return xerrors.Errorf("upload connect public key: %w", err)
	}

	_, _ = fmt.Fprintln(inv.Stderr, "Touch ID protection enabled for workspace connections.")
	return nil
}

// TeardownConnectAuth removes the connect key data file and
// optionally the server-side public key.
func TeardownConnectAuth(cfg config.Root) {
	_ = cfg.ConnectKey().Delete()
}

// ObtainConnectProof signs the current timestamp with the Secure
// Enclave key (triggering Touch ID) and returns the encoded
// proof string suitable for the Coder-Connect-Proof header.
// Returns empty string if Touch ID is not available or no connect
// key is stored.
func ObtainConnectProof(cfg config.Root) (string, error) {
	if !touchid.IsAvailable() {
		return "", nil
	}

	// Read the dataRepresentation from the config directory.
	dataRepB64, err := cfg.ConnectKey().Read()
	if err != nil {
		// No connect key stored — not an error, just no
		// connect-auth available.
		return "", nil
	}

	timestamp := time.Now().Unix()
	tsStr := strconv.FormatInt(timestamp, 10)
	tsB64 := base64.StdEncoding.EncodeToString([]byte(tsStr))

	sigB64, err := touchid.Sign(dataRepB64, tsB64)
	if err != nil {
		return "", xerrors.Errorf("Touch ID sign: %w", err)
	}

	proof := codersdk.ConnectProof{
		Timestamp: timestamp,
		Signature: sigB64,
	}
	encoded, err := codersdk.EncodeConnectProof(proof)
	if err != nil {
		return "", xerrors.Errorf("encode proof: %w", err)
	}
	return encoded, nil
}

// KeyIDFromToken extracts the API key ID (first 10 chars) from a
// session token formatted as "{id}-{secret}".
func KeyIDFromToken(token string) string {
	if token == "" {
		return ""
	}
	parts := strings.SplitN(token, "-", 2)
	return parts[0]
}
