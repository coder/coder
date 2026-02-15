package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/google/uuid"
)

// WebAuthnCredential represents a registered WebAuthn credential.
type WebAuthnCredential struct {
	ID         uuid.UUID  `json:"id" format:"uuid"`
	UserID     uuid.UUID  `json:"user_id" format:"uuid"`
	Name       string     `json:"name"`
	AAGUID     []byte     `json:"aaguid"`
	CreatedAt  time.Time  `json:"created_at" format:"date-time"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty" format:"date-time"`
}

// WebAuthnVerifyResponse is returned after successful assertion
// verification. It contains a short-lived JWT for sensitive operations.
type WebAuthnVerifyResponse struct {
	// JWT is a short-lived token for sensitive operations like SSH
	// and port forwarding. It should be kept in memory only.
	JWT string `json:"jwt"`
}

// BeginWebAuthnRegistration starts the WebAuthn registration ceremony.
func (c *Client) BeginWebAuthnRegistration(ctx context.Context, user string) (protocol.CredentialCreation, error) {
	res, err := c.Request(ctx, http.MethodPost,
		fmt.Sprintf("/api/v2/users/%s/webauthn/register/begin", user), nil)
	if err != nil {
		return protocol.CredentialCreation{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return protocol.CredentialCreation{}, ReadBodyAsError(res)
	}
	var creation protocol.CredentialCreation
	return creation, json.NewDecoder(res.Body).Decode(&creation)
}

// FinishWebAuthnRegistration completes the WebAuthn registration
// ceremony. The attestationBody is the raw attestation response JSON
// from the authenticator.
func (c *Client) FinishWebAuthnRegistration(ctx context.Context, user string, name string, attestationBody json.RawMessage) (WebAuthnCredential, error) {
	path := fmt.Sprintf("/api/v2/users/%s/webauthn/register/finish?name=%s", user, url.QueryEscape(name))
	// Pass as []byte so Request sends the raw JSON directly
	// instead of double-encoding it.
	res, err := c.Request(ctx, http.MethodPost, path, []byte(attestationBody))
	if err != nil {
		return WebAuthnCredential{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return WebAuthnCredential{}, ReadBodyAsError(res)
	}
	var cred WebAuthnCredential
	return cred, json.NewDecoder(res.Body).Decode(&cred)
}

// ListWebAuthnCredentials returns all registered WebAuthn credentials
// for the given user.
func (c *Client) ListWebAuthnCredentials(ctx context.Context, user string) ([]WebAuthnCredential, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/users/%s/webauthn/credentials", user), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var creds []WebAuthnCredential
	return creds, json.NewDecoder(res.Body).Decode(&creds)
}

// DeleteWebAuthnCredential deletes a registered WebAuthn credential.
func (c *Client) DeleteWebAuthnCredential(ctx context.Context, user string, credentialID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete,
		fmt.Sprintf("/api/v2/users/%s/webauthn/credentials/%s", user, credentialID), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// RequestWebAuthnChallenge requests a WebAuthn assertion challenge
// for sensitive operations.
func (c *Client) RequestWebAuthnChallenge(ctx context.Context, user string) (protocol.CredentialAssertion, error) {
	res, err := c.Request(ctx, http.MethodPost,
		fmt.Sprintf("/api/v2/users/%s/webauthn/challenge", user), nil)
	if err != nil {
		return protocol.CredentialAssertion{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return protocol.CredentialAssertion{}, ReadBodyAsError(res)
	}
	var assertion protocol.CredentialAssertion
	return assertion, json.NewDecoder(res.Body).Decode(&assertion)
}

// VerifyWebAuthnChallenge verifies a WebAuthn assertion and returns
// a short-lived JWT for sensitive operations. The assertionBody is
// the raw assertion response JSON from the authenticator.
func (c *Client) VerifyWebAuthnChallenge(ctx context.Context, user string, assertionBody json.RawMessage) (WebAuthnVerifyResponse, error) {
	// Pass as []byte so Request sends the raw JSON directly
	// instead of double-encoding it.
	res, err := c.Request(ctx, http.MethodPost,
		fmt.Sprintf("/api/v2/users/%s/webauthn/verify", user), []byte(assertionBody))
	if err != nil {
		return WebAuthnVerifyResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WebAuthnVerifyResponse{}, ReadBodyAsError(res)
	}
	var resp WebAuthnVerifyResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
