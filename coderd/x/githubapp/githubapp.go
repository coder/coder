// Package githubapp mints GitHub App installation access tokens.
//
// An installation token acts as the GitHub App's bot account on the
// installation's repositories. Minting one takes a short-lived JWT signed
// with the app's private key. Tokens are cached in memory until shortly
// before they expire.
package githubapp

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/sync/singleflight"
	"golang.org/x/xerrors"

	"github.com/coder/quartz"
)

const (
	// DefaultAPIBaseURL is the REST API of github.com.
	DefaultAPIBaseURL = "https://api.github.com"
	// APIBaseURLQueryParam names the query parameter of an MCP server URL
	// that selects the GitHub REST API, matching the built-in GitHub MCP
	// server's option.
	APIBaseURLQueryParam = "api_base_url"

	// MaxRepositories is GitHub's limit on repositories per token.
	MaxRepositories = 500
	// MaxPrivateKeyBytes bounds a pasted private key.
	MaxPrivateKeyBytes = 16 << 10

	// jwtLifetime stays under GitHub's 10 minute limit.
	jwtLifetime = 9 * time.Minute
	// jwtBackdate absorbs clock drift between Coder and GitHub.
	jwtBackdate = 60 * time.Second
	// refreshBefore is how long before expiry a cached token is replaced,
	// so a token handed out stays valid for a whole chat turn.
	refreshBefore = 5 * time.Minute
	// maxCacheAge bounds how long a minted token is reused. GitHub fixes a
	// token's permissions and repositories when it is minted, so changes to
	// the app or its installation apply once the cached token is replaced.
	maxCacheAge = 10 * time.Minute
	// mintTimeout bounds one token request.
	mintTimeout = 30 * time.Second
	// maxResponseBytes bounds GitHub's response body.
	maxResponseBytes = 1 << 20
)

// repositoryNamePattern matches GitHub repository names.
var repositoryNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)

// Credential identifies one GitHub App installation.
type Credential struct {
	// APIBaseURL is the GitHub REST API, such as https://api.github.com
	// or https://HOST/api/v3 for GitHub Enterprise Server.
	APIBaseURL     string
	AppID          int64
	InstallationID int64
	// PrivateKeyPEM is the app's private key. It must never be logged.
	PrivateKeyPEM string
	// Repositories limits tokens to these repository names. Empty means
	// every repository the installation can access.
	Repositories []string
}

// Token is an installation access token.
type Token struct {
	// Token must never be logged.
	Token     string
	ExpiresAt time.Time
}

// Error is an error response from GitHub.
type Error struct {
	StatusCode int
	// Message is GitHub's explanation, such as "Integration not found".
	Message string
}

func (e *Error) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("GitHub returned status %d", e.StatusCode)
	}
	return fmt.Sprintf("GitHub returned status %d: %s", e.StatusCode, e.Message)
}

// ParsePrivateKey parses a PEM encoded RSA private key in PKCS #1 or
// PKCS #8 form, the formats GitHub hands out.
func ParsePrivateKey(raw string) (*rsa.PrivateKey, error) {
	if len(raw) > MaxPrivateKeyBytes {
		return nil, xerrors.New("the private key is too long")
	}
	block, _ := pem.Decode([]byte(strings.TrimSpace(raw)))
	if block == nil {
		return nil, xerrors.New("the private key must be the PEM file GitHub generated, starting with -----BEGIN RSA PRIVATE KEY-----")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, xerrors.New("the private key is not a valid RSA private key")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, xerrors.New("the private key must be an RSA key")
	}
	return key, nil
}

// NormalizeRepositories trims, deduplicates, and sorts repository names.
// "owner/name" is accepted and stored as "name", because a token can only
// cover the installation's own account.
func NormalizeRepositories(repos []string) ([]string, error) {
	out := make([]string, 0, len(repos))
	for _, repo := range repos {
		repo = strings.TrimSpace(repo)
		if repo == "" {
			continue
		}
		if i := strings.LastIndex(repo, "/"); i >= 0 {
			repo = repo[i+1:]
		}
		if !repositoryNamePattern.MatchString(repo) {
			return nil, xerrors.Errorf("%q is not a valid repository name", repo)
		}
		out = append(out, repo)
	}
	slices.Sort(out)
	out = slices.Compact(out)
	if len(out) > MaxRepositories {
		return nil, xerrors.Errorf("at most %d repositories are allowed", MaxRepositories)
	}
	return out, nil
}

// APIBaseURLFromServerURL returns the GitHub REST API an MCP server URL
// selects with its api_base_url query parameter, or DefaultAPIBaseURL.
func APIBaseURLFromServerURL(serverURL string) (string, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return "", xerrors.Errorf("parse MCP server URL: %w", err)
	}
	raw := u.Query().Get(APIBaseURLQueryParam)
	if raw == "" {
		return DefaultAPIBaseURL, nil
	}
	return NormalizeAPIBaseURL(raw)
}

// NormalizeAPIBaseURL validates a GitHub REST API base URL. It must use
// https, except for loopback hosts, which the SSRF guard refuses unless an
// administrator allows them.
func NormalizeAPIBaseURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", xerrors.Errorf("api_base_url is not a valid URL: %w", err)
	}
	if u.Host == "" || u.Opaque != "" {
		return "", xerrors.Errorf("api_base_url must be an absolute URL such as %s", DefaultAPIBaseURL)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", xerrors.New("api_base_url must not contain credentials, a query, or a fragment")
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !isLoopback(u.Hostname()) {
			return "", xerrors.New("api_base_url must use https")
		}
	default:
		return "", xerrors.New("api_base_url must use https")
	}
	u.Host = strings.ToLower(u.Host)
	u.Path = strings.TrimSuffix(u.Path, "/")
	u.RawPath = ""
	return u.String(), nil
}

// APIHost returns the lowercased host of a GitHub REST API base URL.
func APIHost(apiBaseURL string) string {
	u, err := url.Parse(apiBaseURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.IsLoopback()
}

// AppJWT returns a JWT that authenticates as the app for jwtLifetime.
func AppJWT(key *rsa.PrivateKey, appID int64, now time.Time) (string, error) {
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: key},
		(&jose.SignerOptions{}).WithType("JWT"),
	)
	if err != nil {
		return "", xerrors.Errorf("create signer: %w", err)
	}
	claims := jwt.Claims{
		Issuer:   strconv.FormatInt(appID, 10),
		IssuedAt: jwt.NewNumericDate(now.Add(-jwtBackdate)),
		Expiry:   jwt.NewNumericDate(now.Add(jwtLifetime)),
	}
	signed, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		return "", xerrors.Errorf("sign JWT: %w", err)
	}
	return signed, nil
}

// Minter mints installation tokens and caches them per credential.
type Minter struct {
	client *http.Client
	clock  quartz.Clock

	group singleflight.Group
	mu    sync.Mutex
	cache map[string]cachedToken
}

type cachedToken struct {
	Token
	mintedAt time.Time
}

// usable reports whether t can still be handed out at now.
func (t cachedToken) usable(now time.Time) bool {
	return now.Before(t.ExpiresAt.Add(-refreshBefore)) && now.Before(t.mintedAt.Add(maxCacheAge))
}

// NewMinter returns a Minter that calls GitHub with client, which should
// be SSRF guarded.
func NewMinter(client *http.Client, clock quartz.Clock) *Minter {
	if client == nil {
		client = http.DefaultClient
	}
	if clock == nil {
		clock = quartz.NewReal()
	}
	// Redirects could carry the app JWT to another host.
	noRedirects := *client
	noRedirects.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Minter{client: &noRedirects, clock: clock, cache: map[string]cachedToken{}}
}

// Token returns a recently minted installation token that is valid for at
// least a few more minutes, or mints a new one.
func (m *Minter) Token(ctx context.Context, cred Credential) (Token, error) {
	key := cacheKey(cred)
	m.mu.Lock()
	cached, ok := m.cache[key]
	m.mu.Unlock()
	if ok && cached.usable(m.clock.Now()) {
		return cached.Token, nil
	}
	// A caller that gives up must not fail the others sharing the mint.
	ch := m.group.DoChan(key, func() (any, error) {
		tok, err := m.mint(context.WithoutCancel(ctx), cred)
		if err != nil {
			return Token{}, err
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		now := m.clock.Now()
		for k, t := range m.cache {
			if !t.usable(now) {
				delete(m.cache, k)
			}
		}
		m.cache[key] = cachedToken{Token: tok, mintedAt: now}
		return tok, nil
	})
	select {
	case <-ctx.Done():
		return Token{}, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return Token{}, res.Err
		}
		tok, ok := res.Val.(Token)
		if !ok {
			return Token{}, xerrors.New("developer error: mint returned no token")
		}
		return tok, nil
	}
}

// Forget drops the cached token of cred, for example after the credential
// was replaced.
func (m *Minter) Forget(cred Credential) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cache, cacheKey(cred))
}

func (m *Minter) mint(ctx context.Context, cred Credential) (Token, error) {
	key, err := ParsePrivateKey(cred.PrivateKeyPEM)
	if err != nil {
		return Token{}, err
	}
	base, err := NormalizeAPIBaseURL(cred.APIBaseURL)
	if err != nil {
		return Token{}, err
	}
	appJWT, err := AppJWT(key, cred.AppID, m.clock.Now())
	if err != nil {
		return Token{}, err
	}
	var body io.Reader = http.NoBody
	if len(cred.Repositories) > 0 {
		raw, err := json.Marshal(map[string][]string{"repositories": cred.Repositories})
		if err != nil {
			return Token{}, xerrors.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(raw)
	}
	ctx, cancel := context.WithTimeout(ctx, mintTimeout)
	defer cancel()
	endpoint := fmt.Sprintf("%s/app/installations/%d/access_tokens", base, cred.InstallationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return Token{}, xerrors.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if len(cred.Repositories) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := m.client.Do(req)
	if err != nil {
		var netErr net.Error
		if xerrors.As(err, &netErr) && netErr.Timeout() {
			return Token{}, xerrors.Errorf("GitHub did not respond in time: %w", err)
		}
		return Token{}, xerrors.Errorf("reach GitHub: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return Token{}, xerrors.Errorf("read GitHub response: %w", err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		var ghErr struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &ghErr)
		return Token{}, &Error{StatusCode: resp.StatusCode, Message: ghErr.Message}
	}
	var out struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return Token{}, xerrors.Errorf("decode GitHub response: %w", err)
	}
	if out.Token == "" {
		return Token{}, xerrors.New("GitHub returned no token")
	}
	if out.ExpiresAt.IsZero() {
		// GitHub always sets it; assume its documented one hour lifetime.
		out.ExpiresAt = m.clock.Now().Add(time.Hour)
	}
	return Token{Token: out.Token, ExpiresAt: out.ExpiresAt}, nil
}

// cacheKey identifies a credential without keeping the private key.
func cacheKey(cred Credential) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s\x00%d\x00%d\x00%s\x00", cred.APIBaseURL, cred.AppID, cred.InstallationID, strings.Join(cred.Repositories, ","))
	_, _ = io.WriteString(h, cred.PrivateKeyPEM)
	return hex.EncodeToString(h.Sum(nil))
}
