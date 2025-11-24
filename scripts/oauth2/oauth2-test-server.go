package main

import (
	"cmp"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/xerrors"
)

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Error        string `json:"error,omitempty"`
	ErrorDesc    string `json:"error_description,omitempty"`
}

type Config struct {
	ClientID     string
	ClientSecret string
	CodeVerifier string
	State        string
	BaseURL      string
	RedirectURI  string
}

type ServerOptions struct {
	KeepRunning bool
}

func main() {
	var serverOpts ServerOptions
	flag.BoolVar(&serverOpts.KeepRunning, "keep-running", false, "Keep server running after successful authorization")
	flag.Parse()

	config := &Config{
		ClientID:     os.Getenv("CLIENT_ID"),
		ClientSecret: os.Getenv("CLIENT_SECRET"),
		CodeVerifier: os.Getenv("CODE_VERIFIER"),
		State:        os.Getenv("STATE"),
		BaseURL:      cmp.Or(os.Getenv("BASE_URL"), "http://localhost:3000"),
		RedirectURI:  "http://localhost:9876/callback",
	}

	if config.ClientID == "" || config.ClientSecret == "" {
		log.Fatal("CLIENT_ID and CLIENT_SECRET must be set. Run: eval $(./setup-test-app.sh) first")
	}

	if config.CodeVerifier == "" || config.State == "" {
		log.Fatal("CODE_VERIFIER and STATE must be set. Run test-manual-flow.sh to get these values")
	}

	var server *http.Server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
	<title>OAuth2 Test Server</title>
	<style>
		body { font-family: Arial, sans-serif; max-width: 800px; margin: 50px auto; padding: 20px; }
		.status { padding: 20px; margin: 20px 0; border-radius: 5px; }
		.waiting { background: #fff3cd; color: #856404; }
		.success { background: #d4edda; color: #155724; }
		.error { background: #f8d7da; color: #721c24; }
		pre { background: #f5f5f5; padding: 15px; overflow-x: auto; }
		a { color: #0066cc; }
	</style>
</head>
<body>
	<h1>OAuth2 Test Server</h1>
	<div class="status waiting">
		<h2>Waiting for OAuth2 callback...</h2>
		<p>Please authorize the application in your browser.</p>
		<p>Listening on: <code>%s</code></p>
	</div>
</body>
</html>`, config.RedirectURI)
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, html)
	})

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")
		errorParam := r.URL.Query().Get("error")
		errorDesc := r.URL.Query().Get("error_description")

		if errorParam != "" {
			showError(w, fmt.Sprintf("Authorization failed: %s - %s", errorParam, errorDesc))
			return
		}

		if code == "" {
			showError(w, "No authorization code received")
			return
		}

		if state != config.State {
			showError(w, fmt.Sprintf("State mismatch. Expected: %s, Got: %s", config.State, state))
			return
		}

		log.Printf("Received authorization code: %s", code)
		log.Printf("Exchanging code for token...")

		tokenResp, err := exchangeToken(config, code)
		if err != nil {
			showError(w, fmt.Sprintf("Token exchange failed: %v", err))
			return
		}

		showSuccess(w, code, tokenResp, serverOpts)

		if !serverOpts.KeepRunning {
			// Schedule graceful shutdown after giving time for the response to be sent
			go func() {
				time.Sleep(2 * time.Second)
				cancel()
			}()
		}
	})

	server = &http.Server{
		Addr:         ":9876",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("Starting OAuth2 test server on http://localhost:9876")
	log.Printf("Waiting for callback at %s", config.RedirectURI)
	if !serverOpts.KeepRunning {
		log.Printf("Server will shut down automatically after successful authorization")
	}

	// Start server in a goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	log.Printf("Shutting down server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Printf("Server stopped successfully")
}

func exchangeToken(config *Config, code string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("client_id", config.ClientID)
	data.Set("client_secret", config.ClientSecret)
	data.Set("code_verifier", config.CodeVerifier)
	data.Set("redirect_uri", config.RedirectURI)

	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, "POST", config.BaseURL+"/oauth2/tokens", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, xerrors.Errorf("failed to decode response: %w", err)
	}

	if tokenResp.Error != "" {
		return nil, xerrors.Errorf("token error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	return &tokenResp, nil
}

func showError(w http.ResponseWriter, message string) {
	log.Printf("ERROR: %s", message)
	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
	<title>OAuth2 Test - Error</title>
	<style>
		body { font-family: Arial, sans-serif; max-width: 800px; margin: 50px auto; padding: 20px; }
		.status { padding: 20px; margin: 20px 0; border-radius: 5px; }
		.error { background: #f8d7da; color: #721c24; }
		pre { background: #f5f5f5; padding: 15px; overflow-x: auto; }
	</style>
</head>
<body>
	<h1>OAuth2 Test Server - Error</h1>
	<div class="status error">
		<h2>❌ Error</h2>
		<p>%s</p>
	</div>
	<p>Check the server logs for more details.</p>
</body>
</html>`, message)
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = fmt.Fprint(w, html)
}

func showSuccess(w http.ResponseWriter, code string, tokenResp *TokenResponse, opts ServerOptions) {
	log.Printf("SUCCESS: Token exchange completed")
	tokenJSON, _ := json.MarshalIndent(tokenResp, "", "  ")

	serverNote := "The server will shut down automatically in a few seconds."
	if opts.KeepRunning {
		serverNote = "The server will continue running. Press Ctrl+C in the terminal to stop it."
	}

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
	<title>OAuth2 Test - Success</title>
	<style>
		body { font-family: Arial, sans-serif; max-width: 800px; margin: 50px auto; padding: 20px; }
		.status { padding: 20px; margin: 20px 0; border-radius: 5px; }
		.success { background: #d4edda; color: #155724; }
		pre { background: #f5f5f5; padding: 15px; overflow-x: auto; }
		.section { margin: 20px 0; }
		code { background: #e9ecef; padding: 2px 4px; border-radius: 3px; }
	</style>
</head>
<body>
	<h1>OAuth2 Test Server - Success</h1>
	<div class="status success">
		<h2>Authorization Successful!</h2>
		<p>Successfully exchanged authorization code for tokens.</p>
	</div>

	<div class="section">
		<h3>Authorization Code</h3>
		<pre>%s</pre>
	</div>

	<div class="section">
		<h3>Token Response</h3>
		<pre>%s</pre>
	</div>

	<div class="section">
		<h3>Next Steps</h3>
		<p>You can now use the access token to make API requests:</p>
		<pre>curl -H "Coder-Session-Token: %s" %s/api/v2/users/me | jq .</pre>
	</div>

	<div class="section">
		<p><strong>Note:</strong> %s</p>
	</div>
</body>
</html>`, code, string(tokenJSON), tokenResp.AccessToken, cmp.Or(os.Getenv("BASE_URL"), "http://localhost:3000"), serverNote)

	w.Header().Set("Content-Type", "text/html")
	_, _ = fmt.Fprint(w, html)
}
