package cli

import (
	"encoding/json"
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/fido2"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

// ConnectionAuthProvider obtains a short-lived JWT for sensitive
// workspace connections (SSH, port-forward). Implementations can
// use different authentication methods (FIDO2, biometrics, OS
// keychain, etc). The server verifies the JWT regardless of how
// it was obtained — the auth method is a client-side concern.
type ConnectionAuthProvider interface {
	// Name returns a human-readable name for this provider
	// (e.g., "FIDO2", "macOS Keychain").
	Name() string

	// IsAvailable returns true if this provider can be used on
	// the current system (e.g., helper binary installed, hardware
	// present).
	IsAvailable() bool

	// HasCredentials checks whether the user has set up this auth
	// method on the server (e.g., registered a FIDO2 key).
	HasCredentials(inv *serpent.Invocation, client *codersdk.Client) bool

	// ObtainToken performs the auth ceremony and returns a JWT.
	// This may prompt the user (e.g., "touch your security key").
	ObtainToken(inv *serpent.Invocation, client *codersdk.Client) (string, error)
}

// FIDO2AuthProvider implements ConnectionAuthProvider using a FIDO2
// hardware security key via the coder-fido2 helper binary.
type FIDO2AuthProvider struct{}

func (FIDO2AuthProvider) Name() string { return "FIDO2" }

func (FIDO2AuthProvider) IsAvailable() bool {
	return fido2.IsHelperInstalled()
}

func (FIDO2AuthProvider) HasCredentials(inv *serpent.Invocation, client *codersdk.Client) bool {
	creds, err := client.ListWebAuthnCredentials(inv.Context(), codersdk.Me)
	if err != nil {
		return false
	}
	return len(creds) > 0
}

func (FIDO2AuthProvider) ObtainToken(inv *serpent.Invocation, client *codersdk.Client) (string, error) {
	assertion, err := client.RequestWebAuthnChallenge(inv.Context(), codersdk.Me)
	if err != nil {
		return "", xerrors.Errorf("request WebAuthn challenge: %w", err)
	}

	assertionJSON, err := json.Marshal(assertion)
	if err != nil {
		return "", xerrors.Errorf("marshal assertion options: %w", err)
	}

	origin := client.URL.String()
	responseJSON, err := fido2RunWithRetry(inv, fido2.RunAssert, assertionJSON, origin)
	if err != nil {
		return "", xerrors.Errorf("FIDO2 assertion: %w", err)
	}

	_, _ = fmt.Fprintln(inv.Stderr, "Touch detected! Verifying...")

	resp, err := client.VerifyWebAuthnChallenge(inv.Context(), codersdk.Me, responseJSON)
	if err != nil {
		return "", xerrors.Errorf("verify WebAuthn challenge: %w", err)
	}

	return resp.JWT, nil
}

// fido2RunWithRetry is defined in webauthn.go and shared here.
// (It's in the same package so no import needed.)

// defaultConnectionAuthProviders returns the list of providers
// tried in order. The first available provider with credentials
// is used. Add new providers here.
func defaultConnectionAuthProviders() []ConnectionAuthProvider {
	return []ConnectionAuthProvider{
		FIDO2AuthProvider{},
		// Future: MacOSKeychainProvider{}, BiometricProvider{}, etc.
	}
}

// ObtainConnectionJWT tries each registered ConnectionAuthProvider
// in order and returns a JWT from the first one that is available
// and has credentials. Returns empty string if no provider applies
// (user has no credentials or no provider is available).
func ObtainConnectionJWT(inv *serpent.Invocation, client *codersdk.Client) (string, error) {
	for _, p := range defaultConnectionAuthProviders() {
		if !p.IsAvailable() {
			continue
		}
		if !p.HasCredentials(inv, client) {
			continue
		}
		_, _ = fmt.Fprintf(inv.Stderr, "Authenticating with %s...\n", p.Name())
		token, err := p.ObtainToken(inv, client)
		if err != nil {
			return "", xerrors.Errorf("%s authentication: %w", p.Name(), err)
		}
		return token, nil
	}
	return "", nil
}

// promptConnectionAuth is a convenience for commands that need
// connection auth but want to show a user-friendly message when
// it fails. Used by ssh and port-forward.
func promptConnectionAuth(inv *serpent.Invocation, client *codersdk.Client) string {
	jwt, err := ObtainConnectionJWT(inv, client)
	if err != nil {
		cliui.Warnf(inv.Stderr, "Connection authentication failed: %v", err)
		return ""
	}
	return jwt
}
