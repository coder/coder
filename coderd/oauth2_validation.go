package coderd

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// RFC 7591 validation functions for Dynamic Client Registration

// validateClientRegistrationRequest validates the entire registration request
func validateClientRegistrationRequest(req codersdk.OAuth2ClientRegistrationRequest) error {
	// Validate redirect URIs - required for authorization code flow
	if len(req.RedirectURIs) == 0 {
		return xerrors.New("redirect_uris is required for authorization code flow")
	}

	if err := validateRedirectURIs(req.RedirectURIs); err != nil {
		return xerrors.Errorf("invalid redirect_uris: %w", err)
	}

	// Validate grant types if specified
	if len(req.GrantTypes) > 0 {
		if err := validateGrantTypes(req.GrantTypes); err != nil {
			return xerrors.Errorf("invalid grant_types: %w", err)
		}
	}

	// Validate response types if specified
	if len(req.ResponseTypes) > 0 {
		if err := validateResponseTypes(req.ResponseTypes); err != nil {
			return xerrors.Errorf("invalid response_types: %w", err)
		}
	}

	// Validate token endpoint auth method if specified
	if req.TokenEndpointAuthMethod != "" {
		if err := validateTokenEndpointAuthMethod(req.TokenEndpointAuthMethod); err != nil {
			return xerrors.Errorf("invalid token_endpoint_auth_method: %w", err)
		}
	}

	// Validate URI fields
	if req.ClientURI != "" {
		if err := validateURIField(req.ClientURI, "client_uri"); err != nil {
			return err
		}
	}

	if req.LogoURI != "" {
		if err := validateURIField(req.LogoURI, "logo_uri"); err != nil {
			return err
		}
	}

	if req.TOSURI != "" {
		if err := validateURIField(req.TOSURI, "tos_uri"); err != nil {
			return err
		}
	}

	if req.PolicyURI != "" {
		if err := validateURIField(req.PolicyURI, "policy_uri"); err != nil {
			return err
		}
	}

	if req.JWKSURI != "" {
		if err := validateURIField(req.JWKSURI, "jwks_uri"); err != nil {
			return err
		}
	}

	return nil
}

// validateRedirectURIs validates redirect URIs according to RFC 7591
func validateRedirectURIs(uris []string) error {
	if len(uris) == 0 {
		return xerrors.New("at least one redirect URI is required")
	}

	for i, uriStr := range uris {
		if uriStr == "" {
			return xerrors.Errorf("redirect URI at index %d cannot be empty", i)
		}

		uri, err := url.Parse(uriStr)
		if err != nil {
			return xerrors.Errorf("redirect URI at index %d is not a valid URL: %w", i, err)
		}

		// Validate schemes: allow http/https and custom schemes for native apps
		if uri.Scheme == "" {
			return xerrors.Errorf("redirect URI at index %d must have a scheme", i)
		}

		// For http/https schemes, enforce security rules
		if uri.Scheme == "http" || uri.Scheme == "https" {
			// For production, enforce HTTPS except for localhost
			if uri.Scheme == "http" {
				if !isLocalhost(uri.Hostname()) {
					return xerrors.Errorf("redirect URI at index %d must use https scheme for non-localhost URLs", i)
				}
			}
		}
		// Custom schemes are allowed for native applications (RFC 7591)

		// Prevent URI fragments (RFC 6749 section 3.1.2)
		// Check for both explicit fragments and empty fragments (#)
		if uri.Fragment != "" || strings.Contains(uriStr, "#") {
			return xerrors.Errorf("redirect URI at index %d must not contain a fragment component", i)
		}
	}

	return nil
}

// validateGrantTypes validates OAuth2 grant types
func validateGrantTypes(grantTypes []string) error {
	validGrants := []string{
		string(codersdk.OAuth2ProviderGrantTypeAuthorizationCode),
		string(codersdk.OAuth2ProviderGrantTypeRefreshToken),
		// Add more grant types as they are implemented
		// "client_credentials",
		// "urn:ietf:params:oauth:grant-type:device_code",
	}

	for _, grant := range grantTypes {
		if !slices.Contains(validGrants, grant) {
			return xerrors.Errorf("unsupported grant type: %s", grant)
		}
	}

	// Ensure authorization_code is present if redirect_uris are specified
	hasAuthCode := slices.Contains(grantTypes, string(codersdk.OAuth2ProviderGrantTypeAuthorizationCode))
	if !hasAuthCode {
		return xerrors.New("authorization_code grant type is required when redirect_uris are specified")
	}

	return nil
}

// validateResponseTypes validates OAuth2 response types
func validateResponseTypes(responseTypes []string) error {
	validResponses := []string{
		string(codersdk.OAuth2ProviderResponseTypeCode),
		// Add more response types as they are implemented
	}

	for _, responseType := range responseTypes {
		if !slices.Contains(validResponses, responseType) {
			return xerrors.Errorf("unsupported response type: %s", responseType)
		}
	}

	return nil
}

// validateTokenEndpointAuthMethod validates token endpoint authentication method
func validateTokenEndpointAuthMethod(method string) error {
	validMethods := []string{
		"client_secret_post",
		"client_secret_basic",
		// Add more methods as they are implemented
		// "private_key_jwt",
		// "client_secret_jwt",
		// "none", // for public clients
	}

	if !slices.Contains(validMethods, method) {
		return xerrors.Errorf("unsupported token endpoint auth method: %s", method)
	}

	return nil
}

// validateURIField validates a URI field
func validateURIField(uriStr, fieldName string) error {
	if uriStr == "" {
		return nil // Empty URIs are allowed for optional fields
	}

	uri, err := url.Parse(uriStr)
	if err != nil {
		return xerrors.Errorf("invalid %s: %w", fieldName, err)
	}

	// Require absolute URLs with scheme
	if !uri.IsAbs() {
		return xerrors.Errorf("%s must be an absolute URL", fieldName)
	}

	// Only allow http/https schemes
	if uri.Scheme != "http" && uri.Scheme != "https" {
		return xerrors.Errorf("%s must use http or https scheme", fieldName)
	}

	// For production, prefer HTTPS
	// Note: we allow HTTP for localhost but prefer HTTPS for production
	// This could be made configurable in the future

	return nil
}

// applyRegistrationDefaults applies default values to registration request
func applyRegistrationDefaults(req codersdk.OAuth2ClientRegistrationRequest) codersdk.OAuth2ClientRegistrationRequest {
	// Apply grant type defaults
	if len(req.GrantTypes) == 0 {
		req.GrantTypes = []string{
			string(codersdk.OAuth2ProviderGrantTypeAuthorizationCode),
			string(codersdk.OAuth2ProviderGrantTypeRefreshToken),
		}
	}

	// Apply response type defaults
	if len(req.ResponseTypes) == 0 {
		req.ResponseTypes = []string{
			string(codersdk.OAuth2ProviderResponseTypeCode),
		}
	}

	// Apply token endpoint auth method default
	if req.TokenEndpointAuthMethod == "" {
		req.TokenEndpointAuthMethod = "client_secret_basic"
	}

	// Apply client name default if not provided
	if req.ClientName == "" {
		req.ClientName = "Dynamically Registered Client"
	}

	return req
}

// determineClientType determines if client is public or confidential
func determineClientType(_ codersdk.OAuth2ClientRegistrationRequest) string {
	// For now, default to confidential
	// In the future, we might detect based on:
	// - token_endpoint_auth_method == "none" -> public
	// - application_type == "native" -> might be public
	// - Other heuristics
	return "confidential"
}

// isLocalhost checks if hostname is localhost
func isLocalhost(hostname string) bool {
	return hostname == "localhost" ||
		hostname == "127.0.0.1" ||
		hostname == "::1" ||
		strings.HasSuffix(hostname, ".localhost")
}

// generateClientName generates a client name if not provided
func generateClientName(req codersdk.OAuth2ClientRegistrationRequest) string {
	if req.ClientName != "" {
		// Ensure client name fits database constraint (varchar(64))
		if len(req.ClientName) > 64 {
			// Preserve uniqueness by including a hash of the original name
			hash := fmt.Sprintf("%x", sha256.Sum256([]byte(req.ClientName)))[:8]
			maxPrefix := 64 - 1 - len(hash) // 1 for separator
			return req.ClientName[:maxPrefix] + "-" + hash
		}
		return req.ClientName
	}

	// Try to derive from client_uri
	if req.ClientURI != "" {
		if uri, err := url.Parse(req.ClientURI); err == nil && uri.Host != "" {
			name := fmt.Sprintf("Client (%s)", uri.Host)
			if len(name) > 64 {
				return name[:64]
			}
			return name
		}
	}

	// Try to derive from first redirect URI
	if len(req.RedirectURIs) > 0 {
		if uri, err := url.Parse(req.RedirectURIs[0]); err == nil && uri.Host != "" {
			name := fmt.Sprintf("Client (%s)", uri.Host)
			if len(name) > 64 {
				return name[:64]
			}
			return name
		}
	}

	return "Dynamically Registered Client"
}
