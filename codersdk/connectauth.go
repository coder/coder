package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	// ConnectProofHeader is the HTTP header used to transmit the
	// Secure Enclave ECDSA proof for workspace connections.
	ConnectProofHeader = "Coder-Connect-Proof"

	// ConnectAuthRequiredHeader is set by the server in 403
	// responses to signal that connect-auth is required. The
	// client checks this header instead of parsing the error
	// message string.
	ConnectAuthRequiredHeader = "Coder-Connect-Auth-Required"

	// ConnectAuthEndpointSSH is the endpoint category for SSH /
	// workspace coordination (used by `coder ssh`).
	ConnectAuthEndpointSSH = "ssh"
	// ConnectAuthEndpointPortForward is the endpoint category for
	// port forwarding.
	ConnectAuthEndpointPortForward = "port-forward"
	// ConnectAuthEndpointApps is the endpoint category for
	// workspace applications.
	ConnectAuthEndpointApps = "apps"
)

// UpdateConnectPublicKeyRequest sets the ECDSA P-256 public key
// used for connect-auth proofs on an API key.
type UpdateConnectPublicKeyRequest struct {
	// PublicKey is the base64-encoded raw 65-byte uncompressed EC
	// point (0x04 || X || Y) from SecKeyCopyExternalRepresentation.
	PublicKey string `json:"public_key"`
}

// ConnectProof is sent by the CLI in the Coder-Connect-Proof header
// as JSON. It contains a signed Unix timestamp proving possession of
// the Secure Enclave private key.
type ConnectProof struct {
	// Timestamp is the Unix timestamp (seconds) that was signed.
	Timestamp int64 `json:"timestamp"`
	// Signature is the base64-encoded DER-encoded ECDSA signature
	// over SHA-256(timestamp_string).
	Signature string `json:"signature"`
}

// UpdateAPIKeyConnectPublicKey sets the connect public key on an
// API key. The key is a base64-encoded raw 65-byte ECDSA P-256
// public key.
func (c *Client) UpdateAPIKeyConnectPublicKey(ctx context.Context, user, keyID string, req UpdateConnectPublicKeyRequest) error {
	res, err := c.Request(ctx, http.MethodPut,
		fmt.Sprintf("/api/v2/users/%s/keys/%s/connect-key", user, keyID),
		req,
	)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// DeleteAPIKeyConnectPublicKey removes the connect public key from
// an API key, disabling connect-auth for that key.
func (c *Client) DeleteAPIKeyConnectPublicKey(ctx context.Context, user, keyID string) error {
	res, err := c.Request(ctx, http.MethodDelete,
		fmt.Sprintf("/api/v2/users/%s/keys/%s/connect-key", user, keyID),
		nil,
	)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// EncodeConnectProof serializes a ConnectProof to JSON for use in
// the Coder-Connect-Proof HTTP header.
func EncodeConnectProof(proof ConnectProof) (string, error) {
	b, err := json.Marshal(proof)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DecodeConnectProof deserializes a ConnectProof from the
// Coder-Connect-Proof HTTP header value.
func DecodeConnectProof(s string) (ConnectProof, error) {
	var proof ConnectProof
	err := json.Unmarshal([]byte(s), &proof)
	return proof, err
}
