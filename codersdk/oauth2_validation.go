package codersdk

import (
	"net/url"
	"slices"
	"strings"

	"golang.org/x/xerrors"
)

// RFC 7591 validation functions for Dynamic Client Registration

func (req *OAuth2ClientRegistrationRequest) Validate() error {
	// Validate redirect URIs - required for authorization code flow
	if len(req.RedirectURIs) == 0 {
		return xerrors.New("redirect_uris is required for authorization code flow")
	}

	if err := validateRedirectURIs(req.RedirectURIs, req.TokenEndpointAuthMethod); err != nil {
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

// validateRedirectURIs validates redirect URIs according to RFC 7591, 8252
func validateRedirectURIs(uris []string, tokenEndpointAuthMethod OAuth2TokenEndpointAuthMethod) error {
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

		// Validate schemes according to RFC requirements
		if uri.Scheme == "" {
			return xerrors.Errorf("redirect URI at index %d must have a scheme", i)
		}

		// Handle special URNs (RFC 6749 section 3.1.2.1)
		if uri.Scheme == "urn" {
			// Allow the out-of-band redirect URI for native apps
			if uriStr == "urn:ietf:wg:oauth:2.0:oob" {
				continue // This is valid for native apps
			}
			// Other URNs are not standard for OAuth2
			return xerrors.Errorf("redirect URI at index %d uses unsupported URN scheme", i)
		}

		// Block dangerous schemes for security (not allowed by RFCs for OAuth2)
		dangerousSchemes := []string{"javascript", "data", "file", "ftp"}
		for _, dangerous := range dangerousSchemes {
			if strings.EqualFold(uri.Scheme, dangerous) {
				return xerrors.Errorf("redirect URI at index %d uses dangerous scheme %s which is not allowed", i, dangerous)
			}
		}

		// Determine if this is a public client based on token endpoint auth method
		isPublicClient := tokenEndpointAuthMethod == OAuth2TokenEndpointAuthMethodNone

		// Handle different validation for public vs confidential clients
		if uri.Scheme == "http" || uri.Scheme == "https" {
			// HTTP/HTTPS validation (RFC 8252 section 7.3)
			if uri.Scheme == "http" {
				if isPublicClient {
					// For public clients, only allow loopback (RFC 8252)
					if !isLoopbackAddress(uri.Hostname()) {
						return xerrors.Errorf("redirect URI at index %d: public clients may only use http with loopback addresses (127.0.0.1, ::1, localhost)", i)
					}
				} else {
					// For confidential clients, allow localhost for development
					if !isLocalhost(uri.Hostname()) {
						return xerrors.Errorf("redirect URI at index %d must use https scheme for non-localhost URLs", i)
					}
				}
			}
		} else {
			// Custom scheme validation for public clients (RFC 8252 section 7.1)
			if isPublicClient {
				// For public clients, custom schemes should follow RFC 8252 recommendations
				// Should be reverse domain notation based on domain under their control
				if !isValidCustomScheme(uri.Scheme) {
					return xerrors.Errorf("redirect URI at index %d: custom scheme %s should use reverse domain notation (e.g. com.example.app)", i, uri.Scheme)
				}
			}
			// For confidential clients, custom schemes are less common but allowed
		}

		// Prevent URI fragments (RFC 6749 section 3.1.2)
		if uri.Fragment != "" || strings.Contains(uriStr, "#") {
			return xerrors.Errorf("redirect URI at index %d must not contain a fragment component", i)
		}
	}

	return nil
}

// validateGrantTypes validates OAuth2 grant types
func validateGrantTypes(grantTypes []OAuth2ProviderGrantType) error {
	for _, grant := range grantTypes {
		if !grant.Valid() {
			return xerrors.Errorf("unsupported grant type: %s", grant)
		}
	}

	// Ensure authorization_code is present if redirect_uris are specified
	hasAuthCode := slices.Contains(grantTypes, OAuth2ProviderGrantTypeAuthorizationCode)
	if !hasAuthCode {
		return xerrors.New("authorization_code grant type is required when redirect_uris are specified")
	}

	return nil
}

// validateResponseTypes validates OAuth2 response types
func validateResponseTypes(responseTypes []OAuth2ProviderResponseType) error {
	for _, responseType := range responseTypes {
		if !responseType.Valid() {
			return xerrors.Errorf("unsupported response type: %s", responseType)
		}
	}

	return nil
}

// validateTokenEndpointAuthMethod validates token endpoint authentication method
func validateTokenEndpointAuthMethod(method OAuth2TokenEndpointAuthMethod) error {
	if !method.Valid() {
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

// isLocalhost checks if hostname is localhost (allows broader development usage)
func isLocalhost(hostname string) bool {
	return hostname == "localhost" ||
		hostname == "127.0.0.1" ||
		hostname == "::1" ||
		strings.HasSuffix(hostname, ".localhost")
}

// isLoopbackAddress checks if hostname is a strict loopback address (RFC 8252)
func isLoopbackAddress(hostname string) bool {
	return hostname == "localhost" ||
		hostname == "127.0.0.1" ||
		hostname == "::1"
}

// isValidCustomScheme validates custom schemes for public clients (RFC 8252)
func isValidCustomScheme(scheme string) bool {
	// For security and RFC compliance, require reverse domain notation
	// Should contain at least one period and not be a well-known scheme
	if !strings.Contains(scheme, ".") {
		return false
	}

	// Block schemes that look like well-known protocols
	wellKnownSchemes := []string{"http", "https", "ftp", "mailto", "tel", "sms"}
	for _, wellKnown := range wellKnownSchemes {
		if strings.EqualFold(scheme, wellKnown) {
			return false
		}
	}

	return true
}
