package gitprovider_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/externalauth/gitprovider"
	"github.com/coder/coder/v2/testutil"
)

func TestGitHubStatusConcurrentIsolation(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v3/repos/owner/repo/")
		parts := strings.Split(path, "/")
		if len(parts) < 2 {
			http.NotFound(w, r)
			return
		}
		number, err := strconv.Atoi(parts[1])
		if err != nil {
			http.Error(w, "invalid PR number", http.StatusBadRequest)
			return
		}

		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		title := fmt.Sprintf("PR %d for %s", number, token)
		user := map[string]any{
			"login":      "author-" + token,
			"avatar_url": "https://example.com/author.png",
		}
		var body any
		var endpoint string
		switch {
		case parts[0] == "issues":
			endpoint = "issue"
			body = map[string]any{
				"title":      title,
				"state":      "open",
				"draft":      false,
				"updated_at": "2026-11-13T12:00:00Z",
				"user":       user,
				"pull_request": map[string]any{
					"merged_at": nil,
				},
			}
		case parts[0] == "pulls" && len(parts) == 3 && parts[2] == "reviews":
			endpoint = "reviews"
			body = []any{}
		case parts[0] == "pulls":
			endpoint = "pull"
			body = map[string]any{
				"number":     number,
				"title":      title,
				"state":      "open",
				"draft":      false,
				"merged":     false,
				"updated_at": "2026-11-13T12:00:00Z",
				"user":       user,
				"head": map[string]any{
					"sha": fmt.Sprintf("%s-%d", token, number),
					"ref": "feature",
				},
			}
		default:
			http.NotFound(w, r)
			return
		}

		etag := fmt.Sprintf(`"%s-%s"`, endpoint, token)
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)

	provider, err := gitprovider.New("github", srv.URL+"/api/v3", srv.Client())
	require.NoError(t, err)
	tokens := []string{"token-a", "token-b"}

	// 40 simultaneous lookups over two tokens and two PRs, first with a cold
	// cache and then with a warm one. Each must return its own token's PR.
	for range 2 {
		ctx := testutil.Context(t, testutil.WaitLong)
		group, ctx := errgroup.WithContext(ctx)
		start := make(chan struct{})
		for i := range 40 {
			token := tokens[i%len(tokens)]
			number := 1 + (i/len(tokens))%2
			ref := gitprovider.PRRef{
				Owner:  "owner",
				Repo:   "repo",
				Number: number,
			}
			group.Go(func() error {
				<-start
				status, err := provider.FetchPullRequestStatus(ctx, token, ref)
				if err != nil {
					return err
				}
				title := fmt.Sprintf("PR %d for %s", number, token)
				if status.PRNumber != number || status.Title != title {
					return xerrors.Errorf("wrong PR status: want %q, got number %d title %q", title, status.PRNumber, status.Title)
				}
				if status.AuthorLogin != "author-"+token {
					return xerrors.Errorf("wrong PR author for %q: %q", token, status.AuthorLogin)
				}
				return nil
			})
		}
		close(start)
		require.NoError(t, group.Wait())
	}
}
