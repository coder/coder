package githubapp_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/githubapp"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestMinterToken(t *testing.T) {
	t.Parallel()

	key := testKey(t)
	clock := quartz.NewMock(t)
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	clock.Set(now)
	fake := newFakeGitHub(t, &key.PublicKey, clock)
	minter := githubapp.NewMinter(fake.srv.Client(), clock)
	cred := githubapp.Credential{
		APIBaseURL:     fake.srv.URL + "/api/v3",
		AppID:          1234,
		InstallationID: 5678,
		PrivateKeyPEM:  pkcs1PEM(key),
		Repositories:   []string{"coder", "docs"},
	}
	ctx := testutil.Context(t, testutil.WaitShort)

	tok, err := minter.Token(ctx, cred)
	require.NoError(t, err)
	require.Equal(t, "ghs_1", tok.Token)
	require.Equal(t, now.Add(time.Hour), tok.ExpiresAt)

	req := fake.lastRequest()
	require.Equal(t, "/api/v3/app/installations/5678/access_tokens", req.path)
	require.Equal(t, []string{"coder", "docs"}, req.repositories)
	// The JWT is signed by the app's key and authenticates as the app for
	// less than GitHub's 10 minute limit.
	require.Equal(t, "1234", req.claims.Issuer)
	require.Equal(t, now.Add(-time.Minute), req.claims.IssuedAt.Time().UTC())
	require.LessOrEqual(t, req.claims.Expiry.Time().Sub(now), 10*time.Minute)
	require.Greater(t, req.claims.Expiry.Time().Sub(now), time.Duration(0))

	// The token is reused for ten minutes, so permission changes on GitHub
	// apply soon even though the token is valid for an hour.
	clock.Advance(9 * time.Minute)
	tok, err = minter.Token(ctx, cred)
	require.NoError(t, err)
	require.Equal(t, "ghs_1", tok.Token)
	require.EqualValues(t, 1, fake.calls.Load())

	clock.Advance(time.Minute)
	tok, err = minter.Token(ctx, cred)
	require.NoError(t, err)
	require.Equal(t, "ghs_2", tok.Token)
	require.EqualValues(t, 2, fake.calls.Load())

	// A different installation is cached separately.
	other := cred
	other.InstallationID = 9
	other.Repositories = nil
	tok, err = minter.Token(ctx, other)
	require.NoError(t, err)
	require.Equal(t, "ghs_3", tok.Token)
	require.Nil(t, fake.lastRequest().repositories)

	minter.Forget(cred)
	tok, err = minter.Token(ctx, cred)
	require.NoError(t, err)
	require.Equal(t, "ghs_4", tok.Token)
}

func TestMinterConcurrentCallsShareOneMint(t *testing.T) {
	t.Parallel()

	key := testKey(t)
	clock := quartz.NewMock(t)
	fake := newFakeGitHub(t, &key.PublicKey, clock)
	minter := githubapp.NewMinter(fake.srv.Client(), clock)
	cred := githubapp.Credential{APIBaseURL: fake.srv.URL, AppID: 1, InstallationID: 2, PrivateKeyPEM: pkcs8PEM(t, key)}
	ctx := testutil.Context(t, testutil.WaitShort)

	var wg sync.WaitGroup
	tokens := make([]string, 10)
	for i := range tokens {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tok, err := minter.Token(ctx, cred)
			assert.NoError(t, err)
			tokens[i] = tok.Token
		}()
	}
	wg.Wait()
	for _, tok := range tokens {
		require.Equal(t, tokens[0], tok)
	}
	require.LessOrEqual(t, fake.calls.Load(), int64(len(tokens)))
	_, err := minter.Token(ctx, cred)
	require.NoError(t, err)
	calls := fake.calls.Load()
	_, err = minter.Token(ctx, cred)
	require.NoError(t, err)
	require.Equal(t, calls, fake.calls.Load(), "a cached token needs no request")
}

func TestMinterErrors(t *testing.T) {
	t.Parallel()

	key := testKey(t)
	otherKey := testKey(t)
	clock := quartz.NewMock(t)
	fake := newFakeGitHub(t, &key.PublicKey, clock)
	minter := githubapp.NewMinter(fake.srv.Client(), clock)
	ctx := testutil.Context(t, testutil.WaitShort)

	t.Run("WrongKey", func(t *testing.T) {
		t.Parallel()
		_, err := minter.Token(ctx, githubapp.Credential{APIBaseURL: fake.srv.URL, AppID: 1, InstallationID: 2, PrivateKeyPEM: pkcs1PEM(otherKey)})
		var ghErr *githubapp.Error
		require.ErrorAs(t, err, &ghErr)
		require.Equal(t, http.StatusUnauthorized, ghErr.StatusCode)
		require.Equal(t, "A JSON web token could not be decoded", ghErr.Message)
	})
	t.Run("UnknownInstallation", func(t *testing.T) {
		t.Parallel()
		_, err := minter.Token(ctx, githubapp.Credential{APIBaseURL: fake.srv.URL, AppID: 1, InstallationID: 404, PrivateKeyPEM: pkcs1PEM(key)})
		var ghErr *githubapp.Error
		require.ErrorAs(t, err, &ghErr)
		require.Equal(t, http.StatusNotFound, ghErr.StatusCode)
		require.Equal(t, "Not Found", ghErr.Message)
	})
	t.Run("InvalidKey", func(t *testing.T) {
		t.Parallel()
		_, err := minter.Token(ctx, githubapp.Credential{APIBaseURL: fake.srv.URL, AppID: 1, InstallationID: 2, PrivateKeyPEM: "not a key"})
		require.ErrorContains(t, err, "PEM")
	})
	t.Run("NoRedirects", func(t *testing.T) {
		t.Parallel()
		_, err := minter.Token(ctx, githubapp.Credential{APIBaseURL: fake.srv.URL + "/redirect", AppID: 1, InstallationID: 2, PrivateKeyPEM: pkcs1PEM(key)})
		var ghErr *githubapp.Error
		require.ErrorAs(t, err, &ghErr)
		require.Equal(t, http.StatusTemporaryRedirect, ghErr.StatusCode)
	})
}

func TestAPIBaseURLFromServerURL(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		url, want, err string
	}{
		{url: "https://coder.example.com/api/experimental/mcp/builtin/github", want: githubapp.DefaultAPIBaseURL},
		{url: "https://coder.example.com/mcp?api_base_url=https%3A%2F%2FGHES.example.com%2Fapi%2Fv3%2F", want: "https://ghes.example.com/api/v3"},
		{url: "https://coder.example.com/mcp?api_base_url=http%3A%2F%2F127.0.0.1%3A8080", want: "http://127.0.0.1:8080"},
		{url: "https://coder.example.com/mcp?api_base_url=http%3A%2F%2Fghes.example.com", err: "https"},
		{url: "https://coder.example.com/mcp?api_base_url=https%3A%2F%2Fu%3Ap%40ghes.example.com", err: "credentials"},
	} {
		got, err := githubapp.APIBaseURLFromServerURL(tc.url)
		if tc.err != "" {
			require.ErrorContains(t, err, tc.err, tc.url)
			continue
		}
		require.NoError(t, err, tc.url)
		require.Equal(t, tc.want, got, tc.url)
	}
	require.Equal(t, "ghes.example.com", githubapp.APIHost("https://GHES.example.com:443/api/v3"))
}

func TestNormalizeRepositories(t *testing.T) {
	t.Parallel()

	got, err := githubapp.NormalizeRepositories([]string{" docs ", "coder/coder", "", "docs"})
	require.NoError(t, err)
	require.Equal(t, []string{"coder", "docs"}, got)

	_, err = githubapp.NormalizeRepositories([]string{"bad name"})
	require.ErrorContains(t, err, "not a valid repository name")
}

func TestParsePrivateKey(t *testing.T) {
	t.Parallel()

	key := testKey(t)
	parsed, err := githubapp.ParsePrivateKey(pkcs1PEM(key))
	require.NoError(t, err)
	require.True(t, key.Equal(parsed))
	parsed, err = githubapp.ParsePrivateKey(pkcs8PEM(t, key))
	require.NoError(t, err)
	require.True(t, key.Equal(parsed))
	_, err = githubapp.ParsePrivateKey("-----BEGIN RSA PRIVATE KEY-----\nAAAA\n-----END RSA PRIVATE KEY-----")
	require.ErrorContains(t, err, "not a valid RSA private key")
}

type fakeRequest struct {
	path         string
	claims       jwt.Claims
	repositories []string
}

type fakeGitHub struct {
	srv   *httptest.Server
	calls atomic.Int64

	mu   sync.Mutex
	last fakeRequest
}

func (f *fakeGitHub) lastRequest() fakeRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.last
}

// newFakeGitHub serves POST /app/installations/{id}/access_tokens like
// GitHub: it verifies the app JWT against pub and rejects installation 404.
func newFakeGitHub(t *testing.T, pub *rsa.PublicKey, clock quartz.Clock) *fakeGitHub {
	t.Helper()
	f := &fakeGitHub{}
	mux := http.NewServeMux()
	mux.HandleFunc("/redirect/", func(rw http.ResponseWriter, r *http.Request) {
		http.Redirect(rw, r, "https://example.com/", http.StatusTemporaryRedirect)
	})
	handler := func(rw http.ResponseWriter, r *http.Request) {
		writeErr := func(status int, msg string) {
			rw.WriteHeader(status)
			_ = json.NewEncoder(rw).Encode(map[string]string{"message": msg})
		}
		if r.Method != http.MethodPost || r.Header.Get("Accept") != "application/vnd.github+json" {
			writeErr(http.StatusBadRequest, "bad request")
			return
		}
		auth := r.Header.Get("Authorization")
		parsed, err := jwt.ParseSigned(auth[len("Bearer "):], []jose.SignatureAlgorithm{jose.RS256})
		if err != nil {
			writeErr(http.StatusUnauthorized, "A JSON web token could not be decoded")
			return
		}
		var claims jwt.Claims
		if err := parsed.Claims(pub, &claims); err != nil {
			writeErr(http.StatusUnauthorized, "A JSON web token could not be decoded")
			return
		}
		if err := claims.ValidateWithLeeway(jwt.Expected{Time: clock.Now()}, 0); err != nil {
			writeErr(http.StatusUnauthorized, "The JWT is expired or not yet valid")
			return
		}
		if claims.Expiry == nil || claims.Expiry.Time().Sub(clock.Now()) > 10*time.Minute {
			writeErr(http.StatusUnauthorized, "'Expiration time' claim ('exp') is too far in the future")
			return
		}
		if r.PathValue("id") == "404" {
			writeErr(http.StatusNotFound, "Not Found")
			return
		}
		var body struct {
			Repositories []string `json:"repositories"`
		}
		if r.ContentLength > 0 {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		n := f.calls.Add(1)
		f.mu.Lock()
		f.last = fakeRequest{path: r.URL.Path, claims: claims, repositories: body.Repositories}
		f.mu.Unlock()
		rw.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(rw).Encode(map[string]any{
			"token":      "ghs_" + itoa(n),
			"expires_at": clock.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	}
	mux.HandleFunc("POST /app/installations/{id}/access_tokens", handler)
	mux.HandleFunc("POST /api/v3/app/installations/{id}/access_tokens", handler)
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key
}

func pkcs1PEM(key *rsa.PrivateKey) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

func pkcs8PEM(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}
