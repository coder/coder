package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/xerrors"
)

const (
	// ANSI color codes
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"
)

type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete,omitempty"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

type ErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

type Config struct {
	ClientID     string
	ClientSecret string
	BaseURL      string
}

func main() {
	config := &Config{
		ClientID:     os.Getenv("CLIENT_ID"),
		ClientSecret: os.Getenv("CLIENT_SECRET"),
		BaseURL:      getEnvOrDefault("BASE_URL", "http://localhost:3000"),
	}

	if config.ClientID == "" || config.ClientSecret == "" {
		log.Fatal("CLIENT_ID and CLIENT_SECRET must be set. Run: eval $(./setup-test-app.sh) first")
	}

	ctx := context.Background()

	// Step 1: Request device code
	_, _ = fmt.Printf("%s=== Step 1: Device Code Request ===%s\n", colorBlue, colorReset)
	deviceResp, err := requestDeviceCode(ctx, config)
	if err != nil {
		log.Fatalf("Failed to get device code: %v", err)
	}

	_, _ = fmt.Printf("%sDevice Code Response:%s\n", colorGreen, colorReset)
	prettyJSON, _ := json.MarshalIndent(deviceResp, "", "  ")
	_, _ = fmt.Printf("%s\n", prettyJSON)
	_, _ = fmt.Println()

	// Step 2: Display user instructions
	_, _ = fmt.Printf("%s=== Step 2: User Authorization ===%s\n", colorYellow, colorReset)
	_, _ = fmt.Printf("Please visit: %s%s%s\n", colorCyan, deviceResp.VerificationURI, colorReset)
	_, _ = fmt.Printf("Enter code: %s%s%s\n", colorPurple, deviceResp.UserCode, colorReset)
	_, _ = fmt.Println()

	if deviceResp.VerificationURIComplete != "" {
		_, _ = fmt.Printf("Or visit the complete URL: %s%s%s\n", colorCyan, deviceResp.VerificationURIComplete, colorReset)
		_, _ = fmt.Println()
	}

	_, _ = fmt.Printf("Waiting for authorization (expires in %d seconds)...\n", deviceResp.ExpiresIn)
	_, _ = fmt.Printf("Polling every %d seconds...\n", deviceResp.Interval)
	_, _ = fmt.Println()

	// Step 3: Poll for token
	_, _ = fmt.Printf("%s=== Step 3: Token Polling ===%s\n", colorBlue, colorReset)
	tokenResp, err := pollForToken(ctx, config, deviceResp)
	if err != nil {
		log.Fatalf("Failed to get access token: %v", err)
	}

	_, _ = fmt.Printf("%s=== Authorization Successful! ===%s\n", colorGreen, colorReset)
	_, _ = fmt.Printf("%sAccess Token Response:%s\n", colorGreen, colorReset)
	prettyTokenJSON, _ := json.MarshalIndent(tokenResp, "", "  ")
	_, _ = fmt.Printf("%s\n", prettyTokenJSON)
	_, _ = fmt.Println()

	// Step 4: Test the access token
	_, _ = fmt.Printf("%s=== Step 4: Testing Access Token ===%s\n", colorBlue, colorReset)
	if err := testAccessToken(ctx, config, tokenResp.AccessToken); err != nil {
		log.Printf("%sWarning: Failed to test access token: %v%s", colorYellow, err, colorReset)
	} else {
		_, _ = fmt.Printf("%sAccess token is valid and working!%s\n", colorGreen, colorReset)
	}

	_, _ = fmt.Println()
	_, _ = fmt.Printf("%sDevice authorization flow completed successfully!%s\n", colorGreen, colorReset)
	_, _ = fmt.Printf("You can now use the access token to make authenticated API requests.\n")
}

func requestDeviceCode(ctx context.Context, config *Config) (*DeviceCodeResponse, error) {
	// Use x/oauth2 clientcredentials config to structure the request
	// clientConfig := &clientcredentials.Config{
	// 	ClientID:     config.ClientID,
	// 	ClientSecret: config.ClientSecret,
	// 	TokenURL:     config.BaseURL + "/oauth2/device", // Device code endpoint (RFC 8628)
	// }

	// Create form data for device code request
	data := url.Values{}
	data.Set("client_id", config.ClientID)

	// Optional: Add scope parameter
	// data.Set("scope", "openid profile")

	// Make the request to the device authorization endpoint
	req, err := http.NewRequestWithContext(ctx, "POST", config.BaseURL+"/oauth2/device", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, xerrors.Errorf("creating request: %w", err)
	}

	// Set up basic auth with client credentials
	req.SetBasicAuth(config.ClientID, config.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, xerrors.Errorf("making request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil {
			return nil, xerrors.Errorf("device code request failed: %s - %s", errResp.Error, errResp.ErrorDescription)
		}
		return nil, xerrors.Errorf("device code request failed with status %d", resp.StatusCode)
	}

	var deviceResp DeviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&deviceResp); err != nil {
		return nil, xerrors.Errorf("decoding response: %w", err)
	}

	return &deviceResp, nil
}

func pollForToken(ctx context.Context, config *Config, deviceResp *DeviceCodeResponse) (*TokenResponse, error) {
	// Use x/oauth2 config for token exchange
	oauth2Config := &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		Endpoint: oauth2.Endpoint{
			TokenURL: config.BaseURL + "/oauth2/token",
		},
	}

	interval := time.Duration(deviceResp.Interval) * time.Second
	if interval < 5*time.Second {
		interval = 5 * time.Second // Minimum polling interval
	}

	deadline := time.Now().Add(time.Duration(deviceResp.ExpiresIn) * time.Second)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return nil, xerrors.New("device code expired")
			}

			_, _ = fmt.Printf("Polling for token...\n")

			// Create token exchange request using device_code grant
			data := url.Values{}
			data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
			data.Set("device_code", deviceResp.DeviceCode)
			data.Set("client_id", config.ClientID)

			req, err := http.NewRequestWithContext(ctx, "POST", oauth2Config.Endpoint.TokenURL, strings.NewReader(data.Encode()))
			if err != nil {
				return nil, xerrors.Errorf("creating token request: %w", err)
			}

			req.SetBasicAuth(config.ClientID, config.ClientSecret)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			client := &http.Client{Timeout: 30 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				_, _ = fmt.Printf("Request error: %v\n", err)
				continue
			}

			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				_ = resp.Body.Close()
				_, _ = fmt.Printf("Decode error: %v\n", err)
				continue
			}
			_ = resp.Body.Close()

			if errorCode, ok := result["error"].(string); ok {
				switch errorCode {
				case "authorization_pending":
					_, _ = fmt.Printf("Authorization pending... continuing to poll\n")
					continue
				case "slow_down":
					_, _ = fmt.Printf("Slow down request - increasing polling interval by 5 seconds\n")
					interval += 5 * time.Second
					ticker.Reset(interval)
					continue
				case "access_denied":
					return nil, xerrors.New("access denied by user")
				case "expired_token":
					return nil, xerrors.New("device code expired")
				default:
					desc := ""
					if errorDesc, ok := result["error_description"].(string); ok {
						desc = " - " + errorDesc
					}
					return nil, xerrors.Errorf("token error: %s%s", errorCode, desc)
				}
			}

			// Success case - convert to TokenResponse
			var tokenResp TokenResponse
			if accessToken, ok := result["access_token"].(string); ok {
				tokenResp.AccessToken = accessToken
			}
			if tokenType, ok := result["token_type"].(string); ok {
				tokenResp.TokenType = tokenType
			}
			if expiresIn, ok := result["expires_in"].(float64); ok {
				tokenResp.ExpiresIn = int(expiresIn)
			}
			if refreshToken, ok := result["refresh_token"].(string); ok {
				tokenResp.RefreshToken = refreshToken
			}
			if scope, ok := result["scope"].(string); ok {
				tokenResp.Scope = scope
			}

			if tokenResp.AccessToken == "" {
				return nil, xerrors.New("no access token in response")
			}

			return &tokenResp, nil
		}
	}
}

func testAccessToken(ctx context.Context, config *Config, accessToken string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", config.BaseURL+"/api/v2/users/me", nil)
	if err != nil {
		return xerrors.Errorf("creating request: %w", err)
	}

	req.Header.Set("Coder-Session-Token", accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return xerrors.Errorf("making request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return xerrors.Errorf("API request failed with status %d", resp.StatusCode)
	}

	var userInfo map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return xerrors.Errorf("decoding response: %w", err)
	}

	_, _ = fmt.Printf("%sAPI Test Response:%s\n", colorGreen, colorReset)
	prettyJSON, _ := json.MarshalIndent(userInfo, "", "  ")
	_, _ = fmt.Printf("%s\n", prettyJSON)

	return nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
