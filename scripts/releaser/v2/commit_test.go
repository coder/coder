package v2 //nolint:testpackage // Tests unexported release helpers.

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_humanizeTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		title string
		want  string
	}{
		{
			name:  "feat_site_scope",
			title: "feat(site): add bar",
			want:  "Dashboard: Add bar",
		},
		{
			name:  "fix_coderd_scope",
			title: "fix(coderd): thing",
			want:  "Server: Thing",
		},
		{
			name:  "fix_agent_scope",
			title: "fix(agent): reconnect",
			want:  "Agent: Reconnect",
		},
		{
			name:  "feat_cli_scope",
			title: "feat(cli): new flag",
			want:  "CLI: New flag",
		},
		{
			name:  "fix_tailnet_scope",
			title: "fix(tailnet): routing issue",
			want:  "Networking: Routing issue",
		},
		{
			name:  "feat_codersdk_scope",
			title: "feat(codersdk): new method",
			want:  "SDK: New method",
		},
		{
			name:  "feat_docs_scope",
			title: "feat(docs): add guide",
			want:  "Documentation: Add guide",
		},
		{
			name:  "fix_enterprise_coderd_scope",
			title: "fix(enterprise/coderd): auth bug",
			want:  "Server: Auth bug",
		},
		{
			name:  "no_scope",
			title: "feat: thing",
			want:  "Thing",
		},
		{
			name:  "non_conventional_title",
			title: "Update README",
			want:  "Update README",
		},
		{
			name:  "breaking_with_bang_unchanged",
			title: "feat!: thing",
			want:  "feat!: thing",
		},
		{
			name:  "breaking_with_scope_and_bang_unchanged",
			title: "feat(site)!: remove old api",
			want:  "feat(site)!: remove old api",
		},
		{
			name:  "unknown_scope_returns_original",
			title: "fix(unknownscope): something",
			want:  "fix(unknownscope): something",
		},
		{
			name:  "agent_agentssh_more_specific",
			title: "fix(agent/agentssh): session bug",
			want:  "Agent SSH: Session bug",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, humanizeTitle(tt.title))
		})
	}
}

func Test_categorizeCommit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		title  string
		labels []string
		want   string
	}{
		{
			name:  "breaking_via_bang_in_title",
			title: "feat!: remove old api",
			want:  "breaking",
		},
		{
			name:  "breaking_via_scoped_bang",
			title: "fix(coderd)!: breaking change",
			want:  "breaking",
		},
		{
			name:   "breaking_via_label",
			title:  "feat(site): add thing",
			labels: []string{"release/breaking"},
			want:   "breaking",
		},
		{
			name:   "security_label",
			title:  "fix(coderd): patch vuln",
			labels: []string{"security"},
			want:   "security",
		},
		{
			name:   "experimental_label",
			title:  "feat(site): new feature",
			labels: []string{"release/experimental"},
			want:   "experimental",
		},
		{
			name:  "feat_prefix",
			title: "feat(site): add bar",
			want:  "feat",
		},
		{
			name:  "fix_prefix",
			title: "fix(coderd): thing",
			want:  "fix",
		},
		{
			name:  "chore_prefix",
			title: "chore: update deps",
			want:  "chore",
		},
		{
			name:  "docs_prefix",
			title: "docs: update readme",
			want:  "docs",
		},
		{
			name:  "refactor_prefix",
			title: "refactor(coderd): simplify",
			want:  "refactor",
		},
		{
			name:  "unknown_prefix",
			title: "yolo: do something",
			want:  "other",
		},
		{
			name:  "no_prefix",
			title: "Update README",
			want:  "other",
		},
		{
			name:   "breaking_label_takes_priority_over_feat",
			title:  "feat(coderd): new api",
			labels: []string{"release/breaking"},
			want:   "breaking",
		},
		{
			name:   "security_takes_priority_over_experimental",
			title:  "fix(coderd): vuln",
			labels: []string{"security", "release/experimental"},
			want:   "security",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, categorizeCommit(tt.title, tt.labels))
		})
	}
}

func Test_commitSortPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		title string
		want  string
	}{
		{
			name:  "space_delimiter",
			title: "feat something",
			want:  "feat",
		},
		{
			name:  "colon_delimiter",
			title: "feat: something",
			want:  "feat",
		},
		{
			name:  "paren_delimiter",
			title: "feat(site): something",
			want:  "feat",
		},
		{
			name:  "no_delimiter",
			title: "single",
			want:  "single",
		},
		{
			name:  "empty_string",
			title: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, commitSortPrefix(tt.title))
		})
	}
}

func Test_parsePRNumbers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		title string
		want  []int
	}{
		{
			name:  "single_pr",
			title: "feat(site): add bar (#123)",
			want:  []int{123},
		},
		{
			name:  "multiple_prs",
			title: "fix (#42) then (#43)",
			want:  []int{42, 43},
		},
		{
			name:  "no_pr_numbers",
			title: "feat(site): add bar",
			want:  nil,
		},
		{
			name:  "cherry_pick_only_matches_parens",
			title: "chore: foo (cherry-pick #42) (#43)",
			want:  []int{43},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parsePRNumbers(tt.title)
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_stripPRRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		title string
		want  string
	}{
		{
			name:  "removes_trailing_pr_ref",
			title: "Dashboard: Add bar (#123)",
			want:  "Dashboard: Add bar",
		},
		{
			name:  "no_pr_ref",
			title: "Dashboard: Add bar",
			want:  "Dashboard: Add bar",
		},
		{
			name:  "multiple_pr_refs_strips_last",
			title: "Foo (#42) (#43)",
			want:  "Foo (#42)",
		},
		{
			name:  "pr_ref_with_whitespace",
			title: "Title  (#999)",
			want:  "Title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, stripPRRef(tt.title))
		})
	}
}

func Test_isDependabot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		title string
		want  bool
	}{
		{
			name:  "contains_dependabot",
			title: "chore: bump dependabot/fetch-metadata (#456)",
			want:  true,
		},
		{
			name:  "chore_deps_prefix",
			title: "chore(deps): bump golang.org/x/net",
			want:  true,
		},
		{
			name:  "normal_title",
			title: "feat(site): add bar (#123)",
			want:  false,
		},
		{
			name:  "case_insensitive_dependabot",
			title: "Bump Dependabot thing",
			want:  true,
		},
		{
			name:  "chore_deps_uppercase",
			title: "Chore(Deps): update things",
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, isDependabot(tt.title))
		})
	}
}
