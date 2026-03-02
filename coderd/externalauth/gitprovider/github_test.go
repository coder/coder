package gitprovider_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/externalauth/gitprovider"
)

func TestGitHubParseRepositoryOrigin(t *testing.T) {
	t.Parallel()
	gp := gitprovider.New("github", "", nil)
	require.NotNil(t, gp)

	tests := []struct {
		name             string
		raw              string
		expectOK         bool
		expectOwner      string
		expectRepo       string
		expectNormalized string
	}{
		{
			name:             "HTTPS URL",
			raw:              "https://github.com/coder/coder",
			expectOK:         true,
			expectOwner:      "coder",
			expectRepo:       "coder",
			expectNormalized: "https://github.com/coder/coder",
		},
		{
			name:             "HTTPS URL with .git",
			raw:              "https://github.com/coder/coder.git",
			expectOK:         true,
			expectOwner:      "coder",
			expectRepo:       "coder",
			expectNormalized: "https://github.com/coder/coder",
		},
		{
			name:             "HTTPS URL with trailing slash",
			raw:              "https://github.com/coder/coder/",
			expectOK:         true,
			expectOwner:      "coder",
			expectRepo:       "coder",
			expectNormalized: "https://github.com/coder/coder",
		},
		{
			name:             "SSH URL",
			raw:              "git@github.com:coder/coder.git",
			expectOK:         true,
			expectOwner:      "coder",
			expectRepo:       "coder",
			expectNormalized: "https://github.com/coder/coder",
		},
		{
			name:             "SSH URL without .git",
			raw:              "git@github.com:coder/coder",
			expectOK:         true,
			expectOwner:      "coder",
			expectRepo:       "coder",
			expectNormalized: "https://github.com/coder/coder",
		},
		{
			name:             "SSH URL with ssh:// prefix",
			raw:              "ssh://git@github.com/coder/coder.git",
			expectOK:         true,
			expectOwner:      "coder",
			expectRepo:       "coder",
			expectNormalized: "https://github.com/coder/coder",
		},
		{
			name:     "GitLab URL does not match",
			raw:      "https://gitlab.com/coder/coder",
			expectOK: false,
		},
		{
			name:     "Empty string",
			raw:      "",
			expectOK: false,
		},
		{
			name:     "Not a URL",
			raw:      "not-a-url",
			expectOK: false,
		},
		{
			name:             "Hyphenated owner and repo",
			raw:              "https://github.com/my-org/my-repo.git",
			expectOK:         true,
			expectOwner:      "my-org",
			expectRepo:       "my-repo",
			expectNormalized: "https://github.com/my-org/my-repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			owner, repo, normalized, ok := gp.ParseRepositoryOrigin(tt.raw)
			assert.Equal(t, tt.expectOK, ok)
			if tt.expectOK {
				assert.Equal(t, tt.expectOwner, owner)
				assert.Equal(t, tt.expectRepo, repo)
				assert.Equal(t, tt.expectNormalized, normalized)
			}
		})
	}
}

func TestGitHubParsePullRequestURL(t *testing.T) {
	t.Parallel()
	gp := gitprovider.New("github", "", nil)
	require.NotNil(t, gp)

	tests := []struct {
		name         string
		raw          string
		expectOK     bool
		expectOwner  string
		expectRepo   string
		expectNumber int
	}{
		{
			name:         "Standard PR URL",
			raw:          "https://github.com/coder/coder/pull/123",
			expectOK:     true,
			expectOwner:  "coder",
			expectRepo:   "coder",
			expectNumber: 123,
		},
		{
			name:         "PR URL with query string",
			raw:          "https://github.com/coder/coder/pull/456?diff=split",
			expectOK:     true,
			expectOwner:  "coder",
			expectRepo:   "coder",
			expectNumber: 456,
		},
		{
			name:         "PR URL with fragment",
			raw:          "https://github.com/coder/coder/pull/789#discussion",
			expectOK:     true,
			expectOwner:  "coder",
			expectRepo:   "coder",
			expectNumber: 789,
		},
		{
			name:     "Not a PR URL",
			raw:      "https://github.com/coder/coder",
			expectOK: false,
		},
		{
			name:     "Issue URL (not PR)",
			raw:      "https://github.com/coder/coder/issues/123",
			expectOK: false,
		},
		{
			name:     "GitLab MR URL",
			raw:      "https://gitlab.com/coder/coder/-/merge_requests/123",
			expectOK: false,
		},
		{
			name:     "Empty string",
			raw:      "",
			expectOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ref, ok := gp.ParsePullRequestURL(tt.raw)
			assert.Equal(t, tt.expectOK, ok)
			if tt.expectOK {
				assert.Equal(t, tt.expectOwner, ref.Owner)
				assert.Equal(t, tt.expectRepo, ref.Repo)
				assert.Equal(t, tt.expectNumber, ref.Number)
			}
		})
	}
}

func TestGitHubNormalizePullRequestURL(t *testing.T) {
	t.Parallel()
	gp := gitprovider.New("github", "", nil)
	require.NotNil(t, gp)

	tests := []struct {
		name     string
		raw      string
		expected string
	}{
		{
			name:     "Already normalized",
			raw:      "https://github.com/coder/coder/pull/123",
			expected: "https://github.com/coder/coder/pull/123",
		},
		{
			name:     "With trailing punctuation",
			raw:      "https://github.com/coder/coder/pull/123).",
			expected: "https://github.com/coder/coder/pull/123",
		},
		{
			name:     "With query string",
			raw:      "https://github.com/coder/coder/pull/123?diff=split",
			expected: "https://github.com/coder/coder/pull/123",
		},
		{
			name:     "With whitespace",
			raw:      "  https://github.com/coder/coder/pull/123  ",
			expected: "https://github.com/coder/coder/pull/123",
		},
		{
			name:     "Not a PR URL",
			raw:      "https://example.com",
			expected: "",
		},
		{
			name:     "Empty string",
			raw:      "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := gp.NormalizePullRequestURL(tt.raw)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGitHubBuildBranchURL(t *testing.T) {
	t.Parallel()
	gp := gitprovider.New("github", "", nil)
	require.NotNil(t, gp)

	tests := []struct {
		name     string
		owner    string
		repo     string
		branch   string
		expected string
	}{
		{
			name:     "Simple branch",
			owner:    "coder",
			repo:     "coder",
			branch:   "main",
			expected: "https://github.com/coder/coder/tree/main",
		},
		{
			name:     "Branch with slash",
			owner:    "coder",
			repo:     "coder",
			branch:   "feat/new-thing",
			expected: "https://github.com/coder/coder/tree/feat%2Fnew-thing",
		},
		{
			name:     "Empty owner",
			owner:    "",
			repo:     "coder",
			branch:   "main",
			expected: "",
		},
		{
			name:     "Empty repo",
			owner:    "coder",
			repo:     "",
			branch:   "main",
			expected: "",
		},
		{
			name:     "Empty branch",
			owner:    "coder",
			repo:     "coder",
			branch:   "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := gp.BuildBranchURL(tt.owner, tt.repo, tt.branch)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGitHubBuildPullRequestURL(t *testing.T) {
	t.Parallel()
	gp := gitprovider.New("github", "", nil)
	require.NotNil(t, gp)

	tests := []struct {
		name     string
		ref      gitprovider.PRRef
		expected string
	}{
		{
			name:     "Valid PR ref",
			ref:      gitprovider.PRRef{Owner: "coder", Repo: "coder", Number: 123},
			expected: "https://github.com/coder/coder/pull/123",
		},
		{
			name:     "Empty owner",
			ref:      gitprovider.PRRef{Owner: "", Repo: "coder", Number: 123},
			expected: "",
		},
		{
			name:     "Empty repo",
			ref:      gitprovider.PRRef{Owner: "coder", Repo: "", Number: 123},
			expected: "",
		},
		{
			name:     "Zero number",
			ref:      gitprovider.PRRef{Owner: "coder", Repo: "coder", Number: 0},
			expected: "",
		},
		{
			name:     "Negative number",
			ref:      gitprovider.PRRef{Owner: "coder", Repo: "coder", Number: -1},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := gp.BuildPullRequestURL(tt.ref)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGitHubEnterpriseURLs(t *testing.T) {
	t.Parallel()
	gp := gitprovider.New("github", "https://ghes.corp.com/api/v3", nil)
	require.NotNil(t, gp)

	t.Run("ParseRepositoryOrigin HTTPS", func(t *testing.T) {
		t.Parallel()
		owner, repo, normalized, ok := gp.ParseRepositoryOrigin("https://ghes.corp.com/org/repo.git")
		assert.True(t, ok)
		assert.Equal(t, "org", owner)
		assert.Equal(t, "repo", repo)
		assert.Equal(t, "https://ghes.corp.com/org/repo", normalized)
	})

	t.Run("ParseRepositoryOrigin SSH", func(t *testing.T) {
		t.Parallel()
		owner, repo, normalized, ok := gp.ParseRepositoryOrigin("git@ghes.corp.com:org/repo.git")
		assert.True(t, ok)
		assert.Equal(t, "org", owner)
		assert.Equal(t, "repo", repo)
		assert.Equal(t, "https://ghes.corp.com/org/repo", normalized)
	})

	t.Run("ParsePullRequestURL", func(t *testing.T) {
		t.Parallel()
		ref, ok := gp.ParsePullRequestURL("https://ghes.corp.com/org/repo/pull/42")
		assert.True(t, ok)
		assert.Equal(t, "org", ref.Owner)
		assert.Equal(t, "repo", ref.Repo)
		assert.Equal(t, 42, ref.Number)
	})

	t.Run("NormalizePullRequestURL", func(t *testing.T) {
		t.Parallel()
		result := gp.NormalizePullRequestURL("https://ghes.corp.com/org/repo/pull/42?x=y")
		assert.Equal(t, "https://ghes.corp.com/org/repo/pull/42", result)
	})

	t.Run("BuildBranchURL", func(t *testing.T) {
		t.Parallel()
		result := gp.BuildBranchURL("org", "repo", "main")
		assert.Equal(t, "https://ghes.corp.com/org/repo/tree/main", result)
	})

	t.Run("BuildPullRequestURL", func(t *testing.T) {
		t.Parallel()
		result := gp.BuildPullRequestURL(gitprovider.PRRef{Owner: "org", Repo: "repo", Number: 42})
		assert.Equal(t, "https://ghes.corp.com/org/repo/pull/42", result)
	})

	t.Run("github.com URLs do not match GHE instance", func(t *testing.T) {
		t.Parallel()
		_, _, _, ok := gp.ParseRepositoryOrigin("https://github.com/coder/coder")
		assert.False(t, ok, "github.com HTTPS URL should not match GHE instance")

		_, _, _, ok = gp.ParseRepositoryOrigin("git@github.com:coder/coder.git")
		assert.False(t, ok, "github.com SSH URL should not match GHE instance")

		_, ok = gp.ParsePullRequestURL("https://github.com/coder/coder/pull/123")
		assert.False(t, ok, "github.com PR URL should not match GHE instance")
	})
}

func TestNewUnsupportedProvider(t *testing.T) {
	t.Parallel()
	gp := gitprovider.New("unsupported", "", nil)
	assert.Nil(t, gp, "unsupported provider type should return nil")
}
