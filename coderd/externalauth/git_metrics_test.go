package externalauth_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/coderdtest/promhelp"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/externalauth/gitprovider"
	"github.com/coder/coder/v2/coderd/promoauth"
	"github.com/coder/coder/v2/codersdk"
)

func TestConfigGitRequestMetrics(t *testing.T) {
	t.Parallel()

	var mode atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		etag := `"unchanged"`
		var body string
		switch r.URL.Path {
		case "/api/v3/repos/owner/repo/issues/1":
			if mode.Load() == 2 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if mode.Load() == 4 {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			etag = fmt.Sprintf(`"issue-%d"`, mode.Load())
			body = `{"title":"PR","state":"open","draft":false,"updated_at":"2026-11-13T12:00:00Z","pull_request":{"merged_at":null}}`
		case "/api/v3/repos/owner/repo/pulls/1":
			if r.Header.Get("Accept") == "application/vnd.github.diff" {
				body = "+added\n"
			} else {
				body = `{"title":"PR","state":"open","draft":false,"merged":false,"updated_at":"2026-11-13T12:00:00Z"}`
			}
		case "/api/v3/repos/owner/repo/pulls/1/reviews":
			if mode.Load() == 3 {
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			body = `[]`
		case "/api/v3/repos/owner/repo/pulls":
			body = `[]`
		case "/api/v3/repos/owner/repo":
			body = `{"default_branch":"main"}`
		case "/api/v3/repos/owner/repo/compare/main...feature":
			body = "+added\n"
		default:
			t.Errorf("unexpected request: %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	reg := prometheus.NewRegistry()
	accessURL, err := url.Parse("https://coder.example.com")
	require.NoError(t, err)
	configs, err := externalauth.ConvertConfig(t.Context(), slog.Logger{}, promoauth.NewFactory(reg), []codersdk.ExternalAuthConfig{{
		ID: "github-test", Type: "github", ClientID: "test-client", APIBaseURL: srv.URL + "/api/v3",
	}}, accessURL, srv.Client())
	require.NoError(t, err)
	require.Len(t, configs, 1)
	provider, err := configs[0].Git()
	require.NoError(t, err)

	// A context client must not replace the external auth config's TLS client.
	ctx := context.WithValue(t.Context(), oauth2.HTTPClient, http.DefaultClient)
	ref := gitprovider.PRRef{Owner: "owner", Repo: "repo", Number: 1}
	_, err = provider.FetchPullRequestStatus(ctx, "test-token", ref)
	require.NoError(t, err)
	_, err = provider.FetchPullRequestStatus(ctx, "test-token", ref)
	require.NoError(t, err)

	count := func(source string, status int) int {
		t.Helper()
		return promhelp.CounterValue(t, reg, "coderd_oauth2_external_requests_total", prometheus.Labels{
			"name": "github-test", "source": source, "status_code": strconv.Itoa(status),
		})
	}
	assert.Equal(t, 1, count("GitProviderIssue", http.StatusOK))
	assert.Equal(t, 1, count("GitProviderIssue", http.StatusNotModified))
	assert.Equal(t, 1, count("GitProviderPullRequest", http.StatusOK))
	assert.Equal(t, 1, count("GitProviderReviews", http.StatusOK))
	assert.Equal(t, 1, count("GitProviderReviews", http.StatusNotModified))

	mode.Store(1)
	_, err = provider.FetchPullRequestStatus(ctx, "test-token", ref)
	require.NoError(t, err)
	assert.Equal(t, 2, count("GitProviderIssue", http.StatusOK))
	assert.Equal(t, 1, count("GitProviderPullRequest", http.StatusNotModified))

	mode.Store(2)
	_, err = provider.FetchPullRequestStatus(ctx, "test-token", ref)
	require.ErrorContains(t, err, "401")
	assert.Equal(t, 1, count("GitProviderIssue", http.StatusUnauthorized))
	mode.Store(3)
	_, err = provider.FetchPullRequestStatus(ctx, "test-token", ref)
	var rateLimitErr *gitprovider.RateLimitError
	require.ErrorAs(t, err, &rateLimitErr)
	assert.Equal(t, 1, count("GitProviderReviews", http.StatusTooManyRequests))

	mode.Store(4)
	pullsBefore := count("GitProviderPullRequest", http.StatusNotModified)
	for range 2 {
		_, err = provider.FetchPullRequestStatus(ctx, "test-token", ref)
		require.NoError(t, err)
	}
	assert.Equal(t, 1, count("GitProviderIssue", http.StatusForbidden))
	assert.Equal(t, pullsBefore+2, count("GitProviderPullRequest", http.StatusNotModified))

	branch := gitprovider.BranchRef{Owner: "owner", Repo: "repo", Branch: "feature"}
	_, err = provider.ResolveBranchPullRequest(ctx, "test-token", branch)
	require.NoError(t, err)
	_, err = provider.FetchPullRequestDiff(ctx, "test-token", ref)
	require.NoError(t, err)
	_, err = provider.FetchBranchDiff(ctx, "test-token", branch)
	require.NoError(t, err)
	for _, source := range []string{"GitProviderListPullRequests", "GitProviderPullRequestDiff", "GitProviderRepository", "GitProviderBranchDiff"} {
		assert.Equal(t, 1, count(source, http.StatusOK), source)
	}
}
