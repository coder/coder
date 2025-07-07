package oauth2provider

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/userpassword"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/site"
)

const (
	// RFC 8628 recommends device codes be at least 160 bits of entropy
	deviceCodeLength = 32 // 256 bits when base32 encoded
	// Default device code expiration time (RFC 8628 suggests 10-15 minutes)
	deviceCodeExpiration = 15 * time.Minute
	// Default polling interval in seconds
	defaultPollingInterval = 5
)

// DeviceAuthorization handles POST /oauth2/device/authorize - RFC 8628 Device Authorization Request
func DeviceAuthorization(db database.Store, accessURL *url.URL) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// RFC 8628 requires form data.
		contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || contentType != "application/x-www-form-urlencoded" {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_request", "Content-Type must be application/x-www-form-urlencoded")
			return
		}

		if err := r.ParseForm(); err != nil {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_request", "Failed to parse form data")
			return
		}

		req := codersdk.OAuth2DeviceAuthorizationRequest{
			ClientID: r.FormValue("client_id"),
			Scope:    r.FormValue("scope"),
			Resource: r.FormValue("resource"),
		}

		// Validate client_id
		clientID, err := uuid.Parse(req.ClientID)
		if err != nil {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_client", "Invalid client_id format")
			return
		}

		// Check if client exists - use system context for public endpoint
		//nolint:gocritic // Using AsSystemOAuth2 for OAuth2 public endpoint
		app, err := db.GetOAuth2ProviderAppByClientID(dbauthz.AsSystemOAuth2(ctx), clientID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_client", "Client not found")
				return
			}
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusInternalServerError, "server_error", "Failed to validate client")
			return
		}

		// Validate resource parameter if provided (RFC 8707)
		if err := validateResourceParameter(req.Resource); err != nil {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_target", "Invalid resource parameter")
			return
		}

		// Generate device code and user code
		deviceCode, err := generateDeviceCode()
		if err != nil {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusInternalServerError, "server_error", "Failed to generate device code")
			return
		}

		userCode, err := generateUserCode()
		if err != nil {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusInternalServerError, "server_error", "Failed to generate user code")
			return
		}

		// Device code is already hashed in the AppSecret
		hashedDeviceCode := deviceCode.Hashed

		// Create verification URIs
		verificationURI := accessURL.ResolveReference(&url.URL{Path: "/oauth2/device/verify"}).String()
		verificationURIComplete := fmt.Sprintf("%s?user_code=%s", verificationURI, userCode)

		// Store device authorization in database
		expiresAt := dbtime.Now().Add(deviceCodeExpiration)

		//nolint:gocritic // Using AsSystemOAuth2 for OAuth2 public endpoint
		deviceCodeRecord, err := db.InsertOAuth2ProviderDeviceCode(dbauthz.AsSystemOAuth2(ctx), database.InsertOAuth2ProviderDeviceCodeParams{
			ID:                      uuid.New(),
			CreatedAt:               dbtime.Now(),
			ExpiresAt:               expiresAt,
			DeviceCodeHash:          []byte(hashedDeviceCode),
			DeviceCodePrefix:        deviceCode.Prefix,
			UserCode:                userCode,
			ClientID:                app.ID,
			VerificationUri:         verificationURI,
			VerificationUriComplete: sql.NullString{String: verificationURIComplete, Valid: true},
			Scope:                   sql.NullString{String: req.Scope, Valid: req.Scope != ""},
			ResourceUri:             sql.NullString{String: req.Resource, Valid: req.Resource != ""},
			PollingInterval:         defaultPollingInterval,
		})
		if err != nil {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusInternalServerError, "server_error", "Failed to create device authorization")
			return
		}

		// Return device authorization response
		response := codersdk.OAuth2DeviceAuthorizationResponse{
			DeviceCode:              deviceCode.Formatted,
			UserCode:                userCode,
			VerificationURI:         verificationURI,
			VerificationURIComplete: verificationURIComplete,
			ExpiresIn:               int64(deviceCodeExpiration.Seconds()),
			Interval:                int64(deviceCodeRecord.PollingInterval),
		}

		httpapi.Write(ctx, rw, http.StatusOK, response)
	}
}

// DeviceVerification handles GET/POST /oauth2/device - Device verification page
func DeviceVerification(db database.Store) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		switch r.Method {
		case http.MethodGet:
			// Show device verification form
			userCode := r.URL.Query().Get("user_code")
			showDeviceVerificationPage(ctx, db, rw, r, userCode)
		case http.MethodPost:
			// Process device verification
			processDeviceVerification(ctx, rw, r, db)
		default:
			http.Error(rw, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func processDeviceVerification(ctx context.Context, rw http.ResponseWriter, r *http.Request, db database.Store) {
	// Parse form data
	if err := r.ParseForm(); err != nil {
		http.Error(rw, "Invalid form data", http.StatusBadRequest)
		return
	}

	// Extract form values
	userCode := r.FormValue("user_code")
	if userCode == "" {
		http.Error(rw, "Missing user_code parameter", http.StatusBadRequest)
		return
	}

	// Get authenticated user
	apiKey := httpmw.APIKey(r)
	if apiKey.UserID == uuid.Nil {
		http.Error(rw, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Find device code by user code
	//nolint:gocritic // Using AsSystemOAuth2 for OAuth2 device verification
	deviceCode, err := db.GetOAuth2ProviderDeviceCodeByUserCode(dbauthz.AsSystemOAuth2(ctx), userCode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(rw, "Invalid or expired user code", http.StatusBadRequest)
			return
		}
		http.Error(rw, "Database error", http.StatusInternalServerError)
		return
	}

	// Check if device code has expired
	if deviceCode.ExpiresAt.Before(dbtime.Now()) {
		http.Error(rw, "User code has expired", http.StatusBadRequest)
		return
	}

	// Check if already authorized or denied
	if deviceCode.Status != database.OAuth2DeviceStatusPending {
		http.Error(rw, "User code has already been processed", http.StatusBadRequest)
		return
	}

	// Determine action (authorize or deny)
	action := r.FormValue("action")
	var status database.OAuth2DeviceStatus
	switch action {
	case "authorize":
		status = database.OAuth2DeviceStatusAuthorized
	case "deny":
		status = database.OAuth2DeviceStatusDenied
	default:
		http.Error(rw, "Invalid action", http.StatusBadRequest)
		return
	}

	// Update device code authorization status
	//nolint:gocritic // Using AsSystemOAuth2 for OAuth2 device verification
	updatedCode, err := db.UpdateOAuth2ProviderDeviceCodeAuthorization(dbauthz.AsSystemOAuth2(ctx), database.UpdateOAuth2ProviderDeviceCodeAuthorizationParams{
		ID:     deviceCode.ID,
		UserID: uuid.NullUUID{UUID: apiKey.UserID, Valid: true},
		Status: status,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Device code was already processed by another request
			http.Error(rw, "User code has already been processed", http.StatusBadRequest)
			return
		}
		http.Error(rw, "Failed to update authorization", http.StatusInternalServerError)
		return
	}

	// Verify the update succeeded by checking the returned status
	if updatedCode.Status != status {
		http.Error(rw, "User code has already been processed", http.StatusBadRequest)
		return
	}

	// Get app information for display
	var appName string
	//nolint:gocritic // Using AsSystemOAuth2 for OAuth2 device verification
	app, err := db.GetOAuth2ProviderAppByID(dbauthz.AsSystemOAuth2(ctx), deviceCode.ClientID)
	if err == nil {
		appName = app.Name
	}

	// Show success page
	if status == database.OAuth2DeviceStatusAuthorized {
		showDeviceAuthorizationSuccess(rw, r, appName)
	} else {
		showDeviceAuthorizationDenied(rw, r, appName)
	}
}

func showDeviceVerificationPage(ctx context.Context, db database.Store, rw http.ResponseWriter, r *http.Request, userCode string) {
	data := site.RenderOAuthDeviceData{
		UserCode: userCode,
	}

	// Try to get app information if user code is provided
	if userCode != "" {
		//nolint:gocritic // Using AsSystemOAuth2 for OAuth2 device verification
		deviceCode, err := db.GetOAuth2ProviderDeviceCodeByUserCode(dbauthz.AsSystemOAuth2(ctx), userCode)
		if err == nil {
			//nolint:gocritic // Using AsSystemOAuth2 for OAuth2 device verification
			app, err := db.GetOAuth2ProviderAppByID(dbauthz.AsSystemOAuth2(ctx), deviceCode.ClientID)
			if err == nil {
				data.AppName = app.Name
				if app.Icon != "" {
					data.AppIcon = app.Icon
				}
			}
		}
	}

	site.RenderOAuthDevicePage(rw, r, data)
}

func showDeviceAuthorizationSuccess(rw http.ResponseWriter, r *http.Request, appName string) {
	data := site.RenderOAuthDeviceResultData{
		AppName: appName,
	}

	site.RenderOAuthDeviceSuccessPage(rw, r, data)
}

func showDeviceAuthorizationDenied(rw http.ResponseWriter, r *http.Request, appName string) {
	data := site.RenderOAuthDeviceResultData{
		AppName: appName,
	}

	site.RenderOAuthDeviceDeniedPage(rw, r, data)
}

// generateDeviceCode generates a cryptographically secure device code
func generateDeviceCode() (AppSecret, error) {
	bytes := make([]byte, deviceCodeLength)
	if _, err := rand.Read(bytes); err != nil {
		return AppSecret{}, xerrors.Errorf("generate device code: %w", err)
	}

	// Use base32 encoding for better readability and URL safety
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(bytes)
	secret := strings.ToLower(encoded)

	// Generate prefix for device codes
	prefix := secret[:8]

	hashed, err := userpassword.Hash(secret)
	if err != nil {
		return AppSecret{}, xerrors.Errorf("hash device code: %w", err)
	}

	return AppSecret{
		Formatted: fmt.Sprintf("cdr_device_%s_%s", prefix, secret),
		Prefix:    prefix,
		Hashed:    hashed,
	}, nil
}

// generateUserCode generates a human-readable user code.
func generateUserCode() (string, error) {
	code, err := cryptorand.StringCharset(cryptorand.Human, 8)
	if err != nil {
		return "", xerrors.Errorf("generate user code: %w", err)
	}

	// Format as XXXX-XXXX for better readability.
	return fmt.Sprintf("%s-%s", code[:4], code[4:]), nil
}
