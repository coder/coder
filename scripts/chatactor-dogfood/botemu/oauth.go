package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// authorizationCodeToken drives the OAuth2 authorization code flow the way
// a first-party bot would after linking a Slack user to a Coder user: the
// user consents once (emulated by POSTing the authorize URL with the user's
// session), and the bot exchanges the code with PKCE.
func authorizationCodeToken(ctx context.Context, accessURL string, appID uuid.UUID, appSecret, userSession, scope string) (token, meta string, err error) {
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return "", "", err
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	stateBytes := make([]byte, 8)
	_, _ = rand.Read(stateBytes)
	stateVal := base64.RawURLEncoding.EncodeToString(stateBytes)

	q := url.Values{}
	q.Set("client_id", appID.String())
	q.Set("response_type", "code")
	q.Set("redirect_uri", callbackURL)
	q.Set("state", stateVal)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	if scope != "" {
		q.Set("scope", scope)
	}
	authURL := accessURL + "/oauth2/authorize?" + q.Encode()

	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Coder-Session-Token", userSession)
	resp, err := noRedirect.Do(req)
	if err != nil {
		return "", "", err
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusTemporaryRedirect {
		return "", "", xerrors.Errorf("authorize: status %d body %s", resp.StatusCode, truncate(string(body), 300))
	}
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		return "", "", err
	}
	if e := loc.Query().Get("error"); e != "" {
		return "", "", xerrors.Errorf("authorize redirected with error=%s description=%q", e, loc.Query().Get("error_description"))
	}
	code := loc.Query().Get("code")
	if code == "" {
		return "", "", xerrors.New("authorize: no code in redirect")
	}
	if loc.Query().Get("state") != stateVal {
		return "", "", xerrors.New("authorize: state mismatch")
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", appID.String())
	form.Set("client_secret", appSecret)
	form.Set("code_verifier", verifier)
	form.Set("redirect_uri", callbackURL)
	treq, err := http.NewRequestWithContext(ctx, http.MethodPost, accessURL+"/oauth2/tokens", strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	treq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tresp, err := http.DefaultClient.Do(treq)
	if err != nil {
		return "", "", err
	}
	tbody, _ := io.ReadAll(tresp.Body)
	_ = tresp.Body.Close()
	if tresp.StatusCode != http.StatusOK {
		return "", "", xerrors.Errorf("token: status %d body %s", tresp.StatusCode, truncate(string(tbody), 300))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(tbody, &tok); err != nil {
		return "", "", err
	}
	if tok.AccessToken == "" {
		return "", "", xerrors.New("token: empty access_token")
	}
	return tok.AccessToken, fmt.Sprintf("token_type=%s expires_in=%d scope=%q", tok.TokenType, tok.ExpiresIn, tok.Scope), nil
}

func runTokens(ctx context.Context, st *state, scopes string) error {
	if st.AppID == uuid.Nil {
		return xerrors.New("run setup first")
	}
	for _, name := range []string{"alice", "bob", "carol"} {
		us := st.Users[name]
		tok, meta, err := authorizationCodeToken(ctx, st.AccessURL, st.AppID, st.AppSecret, us.SessionToken, scopes)
		if err != nil {
			return xerrors.Errorf("token for %s: %w", name, err)
		}
		us.AccessToken = tok
		us.Scopes = scopes
		st.Users[name] = us
		logf("oauth2 token obtained for %s via app slack-bot: %s requested_scope=%q", name, meta, scopes)
	}
	tok, meta, err := authorizationCodeToken(ctx, st.AccessURL, st.NoShareAppID, st.NoShareSecret, st.Users["alice"].SessionToken, noShareScopes)
	if err != nil {
		return xerrors.Errorf("noshare token for alice: %w", err)
	}
	st.AliceNoShareTk = tok
	logf("oauth2 token obtained for alice via app slack-bot-noshare: %s requested_scope=%q", meta, noShareScopes)
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
