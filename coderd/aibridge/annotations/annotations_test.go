package annotations_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridge/annotations"
)

func TestParams(t *testing.T) {
	t.Parallel()

	t.Run("TrimsAndKeepsOnlySetFields", func(t *testing.T) {
		t.Parallel()

		params, err := annotations.Params(annotations.Input{
			LinearIssueIDs: []string{" ENG-1234 ", "  "},
			Repos:          []string{" coder/coder "},
		})
		require.NoError(t, err)
		require.Equal(t, []string{"ENG-1234"}, params.LinearIssueIds)
		require.Equal(t, []string{"coder/coder"}, params.Repos)
		require.Empty(t, params.Branches)
		require.Empty(t, params.GithubPrUrls)
	})

	t.Run("NoFields", func(t *testing.T) {
		t.Parallel()

		_, err := annotations.Params(annotations.Input{Repos: []string{"   "}, LinearIssueIDs: []string{}})
		require.ErrorContains(t, err, "provide at least one of")
	})

	t.Run("ValueTooLong", func(t *testing.T) {
		t.Parallel()

		for name, input := range map[string]annotations.Input{
			"repos":            {Repos: []string{strings.Repeat("a", annotations.MaxValueLength+1)}},
			"branches":         {Branches: []string{strings.Repeat("a", annotations.MaxValueLength+1)}},
			"linear_issue_ids": {LinearIssueIDs: []string{strings.Repeat("a", annotations.MaxValueLength+1)}},
			"github_pr_urls":   {GitHubPRURLs: []string{strings.Repeat("a", annotations.MaxValueLength+1)}},
		} {
			_, err := annotations.Params(input)
			require.ErrorContains(t, err, "256 characters or fewer", "field %q", name)
		}
	})

	t.Run("TooManyItems", func(t *testing.T) {
		t.Parallel()

		issues := make([]string, annotations.MaxValuesPerKey+1)
		for i := range issues {
			issues[i] = "ENG-1"
		}
		_, err := annotations.Params(annotations.Input{LinearIssueIDs: issues})
		require.ErrorContains(t, err, "linear_issue_ids accepts at most 16 values per call")

		branches := make([]string, annotations.MaxValuesPerKey+1)
		for i := range branches {
			branches[i] = "main"
		}
		_, err = annotations.Params(annotations.Input{Branches: branches})
		require.ErrorContains(t, err, "branches accepts at most 16 values per call")

		prs := make([]string, annotations.MaxValuesPerKey+1)
		for i := range prs {
			prs[i] = "https://github.com/coder/coder/pull/1"
		}
		_, err = annotations.Params(annotations.Input{GitHubPRURLs: prs})
		require.ErrorContains(t, err, "github_pr_urls accepts at most 16 values per call")
	})

	t.Run("AcceptsCanonicalPullRequestURLs", func(t *testing.T) {
		t.Parallel()

		params, err := annotations.Params(annotations.Input{GitHubPRURLs: []string{
			"https://github.com/coder/coder/pull/28300",
			"https://github.com/some-org/some.repo_name/pull/1",
		}})
		require.NoError(t, err)
		require.Len(t, params.GithubPrUrls, 2)
	})

	t.Run("RejectsNonPullRequestURLs", func(t *testing.T) {
		t.Parallel()

		for _, url := range []string{
			"javascript:alert(1)",
			"http://github.com/coder/coder/pull/1",
			"https://github.example.com/coder/coder/pull/1",
			"https://github.com.evil.example/coder/coder/pull/1",
			"https://github.com/coder/coder/issues/1",
			"https://github.com/coder/coder/pull/1?x=y",
			"https://github.com/coder/coder/pull/1#files",
			"https://github.com/coder/coder/pull/1/files",
			"https://github.com/coder/coder/pull/abc",
			"https://github.com/coder/pull/1",
		} {
			_, err := annotations.Params(annotations.Input{GitHubPRURLs: []string{url}})
			require.ErrorContains(t, err, "must look like https://github.com/", "url %q", url)
		}
	})
}
