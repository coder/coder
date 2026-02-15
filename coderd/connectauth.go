package coderd

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// connectAuthTimestampTolerance is the maximum allowed difference
	// between the proof timestamp and the server's current time.
	connectAuthTimestampTolerance = 30 * time.Second

	// ecP256UncompressedKeyLen is the length of an uncompressed
	// ECDSA P-256 public key (0x04 || X(32) || Y(32)).
	ecP256UncompressedKeyLen = 65
)

// putAPIKeyConnectPublicKey sets the Secure Enclave public key on
// the authenticated user's API key. This enables connect-auth
// enforcement for workspace connections using this key.
//
// @Summary Set connect public key
// @ID set-connect-public-key
// @Security CoderSessionToken
// @Tags API Keys
// @Accept json
// @Param user path string true "User ID, name, or me"
// @Param keyid path string true "API Key ID"
// @Param request body codersdk.UpdateConnectPublicKeyRequest true "Public key"
// @Success 204
// @Router /users/{user}/keys/{keyid}/connect-key [put]
func (api *API) putAPIKeyConnectPublicKey(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	keyID := chi.URLParam(r, "keyid")

	forKey, err := api.Database.GetAPIKeyByID(ctx, keyID)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	// Reject if a connect key is already set. Once enrolled, the
	// key is immutable for this API key. To get a new connect key,
	// the user must create a new session (coder login), which
	// creates a new API key. This prevents an attacker with a
	// stolen session token from silently replacing the victim's
	// connect key.
	if len(forKey.ConnectPublicKey) > 0 {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "Connect public key is already set on this API key and cannot be changed. Create a new session with 'coder login' to enroll a new key.",
		})
		return
	}

	var req codersdk.UpdateConnectPublicKeyRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	rawPub, err := base64.StdEncoding.DecodeString(req.PublicKey)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid base64 public key.",
			Detail:  err.Error(),
		})
		return
	}

	if err := validateECP256PublicKey(rawPub); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid ECDSA P-256 public key.",
			Detail:  err.Error(),
		})
		return
	}

	err = api.Database.UpdateAPIKeyConnectPublicKey(ctx, database.UpdateAPIKeyConnectPublicKeyParams{
		ID:               forKey.ID,
		ConnectPublicKey: rawPub,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// deleteAPIKeyConnectPublicKey removes the connect public key from
// an API key, disabling connect-auth for that key.
//
// @Summary Delete connect public key
// @ID delete-connect-public-key
// @Security CoderSessionToken
// @Tags API Keys
// @Param user path string true "User ID, name, or me"
// @Param keyid path string true "API Key ID"
// @Success 204
// @Router /users/{user}/keys/{keyid}/connect-key [delete]
func (api *API) deleteAPIKeyConnectPublicKey(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	keyID := chi.URLParam(r, "keyid")

	forKey, err := api.Database.GetAPIKeyByID(ctx, keyID)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	err = api.Database.UpdateAPIKeyConnectPublicKey(ctx, database.UpdateAPIKeyConnectPublicKeyParams{
		ID:               forKey.ID,
		ConnectPublicKey: nil,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// verifyConnectProof checks the Coder-Connect-Proof header against
// the API key's stored public key. Returns nil if the proof is
// valid, or an error describing why it failed.
func verifyConnectProof(header string, pubKeyRaw []byte) error {
	if header == "" {
		return xerrors.New("missing Coder-Connect-Proof header")
	}

	var proof codersdk.ConnectProof
	if err := json.Unmarshal([]byte(header), &proof); err != nil {
		return xerrors.Errorf("malformed connect proof: %w", err)
	}

	// Verify timestamp is within tolerance.
	proofTime := time.Unix(proof.Timestamp, 0)
	diff := time.Since(proofTime)
	if diff < 0 {
		diff = -diff
	}
	if diff > connectAuthTimestampTolerance {
		return xerrors.Errorf("connect proof timestamp out of range: %s drift", diff)
	}

	// Decode the signature.
	sigBytes, err := base64.StdEncoding.DecodeString(proof.Signature)
	if err != nil {
		return xerrors.Errorf("invalid signature encoding: %w", err)
	}

	// Parse the public key.
	pubKey, err := parseRawECP256PublicKey(pubKeyRaw)
	if err != nil {
		return xerrors.Errorf("stored public key invalid: %w", err)
	}

	// The signed data is SHA-256 of the timestamp string.
	tsStr := strconv.FormatInt(proof.Timestamp, 10)
	digest := sha256.Sum256([]byte(tsStr))

	// Verify the ECDSA signature (DER ASN.1 encoded).
	if !ecdsa.VerifyASN1(pubKey, digest[:], sigBytes) {
		return xerrors.New("connect proof signature verification failed")
	}

	return nil
}

// validateECP256PublicKey checks that raw is a valid uncompressed
// EC P-256 public key and that the point is on the curve.
func validateECP256PublicKey(raw []byte) error {
	if len(raw) != ecP256UncompressedKeyLen {
		return fmt.Errorf("expected %d bytes, got %d", ecP256UncompressedKeyLen, len(raw))
	}
	if raw[0] != 0x04 {
		return fmt.Errorf("expected uncompressed point prefix 0x04, got 0x%02x", raw[0])
	}
	x := new(big.Int).SetBytes(raw[1:33])
	y := new(big.Int).SetBytes(raw[33:65])
	if !elliptic.P256().IsOnCurve(x, y) {
		return fmt.Errorf("point is not on the P-256 curve")
	}
	return nil
}

// parseRawECP256PublicKey converts a 65-byte raw uncompressed EC
// point into an *ecdsa.PublicKey.
func parseRawECP256PublicKey(raw []byte) (*ecdsa.PublicKey, error) {
	if err := validateECP256PublicKey(raw); err != nil {
		return nil, err
	}
	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(raw[1:33]),
		Y:     new(big.Int).SetBytes(raw[33:65]),
	}, nil
}

// connectAuthRequired returns true if the given endpoint category
// is in the deployment's connect-auth enforcement list.
func connectAuthRequired(endpoints []string, category string) bool {
	for _, ep := range endpoints {
		if ep == category {
			return true
		}
	}
	return false
}
