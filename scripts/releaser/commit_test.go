package main //nolint:testpackage // Tests unexported release helpers.

import (
	"testing"
)

func TestFindBreakingCommits(t *testing.T) {
	t.Parallel()

	// Metadata for PRs that were labeled release/breaking. #10 is
	// keyed by merge-commit SHA (direct merge), #20 only by PR
	// number (cherry-picked, so the SHA on the release branch
	// differs from the merge commit on main). #99 is labeled but
	// deliberately absent from the commit list, simulating a
	// breaking PR that shipped in an earlier release on the branch.
	prMeta := &prMetadataMaps{
		bySHA: map[string]prMetadata{
			"aaa": {Labels: []string{"release/breaking"}},
		},
		byNumber: map[int]prMetadata{
			10: {Labels: []string{"release/breaking"}},
			20: {Labels: []string{"release/breaking"}},
			99: {Labels: []string{"release/breaking"}},
		},
	}

	commits := []commitEntry{
		{SHA: "aaa", FullSHA: "aaa", Title: "feat: labeled via sha (#10)", PRCount: 10},
		{SHA: "bbb", FullSHA: "bbb", Title: "fix: labeled via pr number (#20)", PRCount: 20},
		{SHA: "ccc", FullSHA: "ccc", Title: "feat(cli)!: bang title (#30)", PRCount: 30},
		{SHA: "ddd", FullSHA: "ddd", Title: "fix: ordinary change (#40)", PRCount: 40},
		{SHA: "eee", FullSHA: "eee", Title: "chore: no pr number"},
	}

	got := findBreakingCommits(commits, prMeta)

	want := []string{"aaa", "bbb", "ccc"}
	if len(got) != len(want) {
		t.Fatalf("got %d breaking commits, want %d: %+v", len(got), len(want), got)
	}
	for i, c := range got {
		if c.SHA != want[i] {
			t.Fatalf("breaking[%d] = %s, want %s", i, c.SHA, want[i])
		}
	}
}

func TestFindBreakingCommitsWithoutMetadata(t *testing.T) {
	t.Parallel()

	// With no PR metadata (gh unavailable), only "!" titles are
	// detected.
	prMeta := &prMetadataMaps{
		bySHA:    map[string]prMetadata{},
		byNumber: map[int]prMetadata{},
	}
	commits := []commitEntry{
		{SHA: "aaa", FullSHA: "aaa", Title: "feat!: bang (#1)", PRCount: 1},
		{SHA: "bbb", FullSHA: "bbb", Title: "feat: plain (#2)", PRCount: 2},
	}

	got := findBreakingCommits(commits, prMeta)
	if len(got) != 1 || got[0].SHA != "aaa" {
		t.Fatalf("got %+v, want only aaa", got)
	}

	if got := findBreakingCommits(nil, prMeta); got != nil {
		t.Fatalf("got %+v for no commits, want nil", got)
	}
}
