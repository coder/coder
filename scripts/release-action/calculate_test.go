package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_versionIsLess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b version
		want bool
	}{
		{
			name: "major_less",
			a:    version{major: 1, minor: 0, patch: 0, rc: -1, original: "v1.0.0"},
			b:    version{major: 2, minor: 0, patch: 0, rc: -1, original: "v2.0.0"},
			want: true,
		},
		{
			name: "major_greater",
			a:    version{major: 3, minor: 0, patch: 0, rc: -1, original: "v3.0.0"},
			b:    version{major: 2, minor: 0, patch: 0, rc: -1, original: "v2.0.0"},
			want: false,
		},
		{
			name: "minor_less",
			a:    version{major: 2, minor: 1, patch: 0, rc: -1, original: "v2.1.0"},
			b:    version{major: 2, minor: 2, patch: 0, rc: -1, original: "v2.2.0"},
			want: true,
		},
		{
			name: "minor_greater",
			a:    version{major: 2, minor: 5, patch: 0, rc: -1, original: "v2.5.0"},
			b:    version{major: 2, minor: 2, patch: 0, rc: -1, original: "v2.2.0"},
			want: false,
		},
		{
			name: "patch_less",
			a:    version{major: 2, minor: 1, patch: 0, rc: -1, original: "v2.1.0"},
			b:    version{major: 2, minor: 1, patch: 3, rc: -1, original: "v2.1.3"},
			want: true,
		},
		{
			name: "patch_greater",
			a:    version{major: 2, minor: 1, patch: 5, rc: -1, original: "v2.1.5"},
			b:    version{major: 2, minor: 1, patch: 3, rc: -1, original: "v2.1.3"},
			want: false,
		},
		{
			name: "rc_less_than_non_rc",
			a:    version{major: 2, minor: 1, patch: 0, rc: 5, original: "v2.1.0-rc.5"},
			b:    version{major: 2, minor: 1, patch: 0, rc: -1, original: "v2.1.0"},
			want: true,
		},
		{
			name: "non_rc_not_less_than_rc",
			a:    version{major: 2, minor: 1, patch: 0, rc: -1, original: "v2.1.0"},
			b:    version{major: 2, minor: 1, patch: 0, rc: 5, original: "v2.1.0-rc.5"},
			want: false,
		},
		{
			name: "equal_non_rc",
			a:    version{major: 2, minor: 1, patch: 0, rc: -1, original: "v2.1.0"},
			b:    version{major: 2, minor: 1, patch: 0, rc: -1, original: "v2.1.0"},
			want: false,
		},
		{
			name: "equal_rc",
			a:    version{major: 2, minor: 1, patch: 0, rc: 3, original: "v2.1.0-rc.3"},
			b:    version{major: 2, minor: 1, patch: 0, rc: 3, original: "v2.1.0-rc.3"},
			want: false,
		},
		{
			name: "rc_ordering",
			a:    version{major: 2, minor: 1, patch: 0, rc: 1, original: "v2.1.0-rc.1"},
			b:    version{major: 2, minor: 1, patch: 0, rc: 3, original: "v2.1.0-rc.3"},
			want: true,
		},
		{
			name: "rc_ordering_reverse",
			a:    version{major: 2, minor: 1, patch: 0, rc: 3, original: "v2.1.0-rc.3"},
			b:    version{major: 2, minor: 1, patch: 0, rc: 1, original: "v2.1.0-rc.1"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, versionIsLess(tt.a, tt.b))
		})
	}
}

func Test_findLatestRC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tags []version
		want version
	}{
		{
			name: "empty_list",
			tags: nil,
			want: version{},
		},
		{
			name: "no_rcs",
			tags: []version{
				{major: 2, minor: 1, patch: 0, rc: -1, original: "v2.1.0"},
				{major: 2, minor: 2, patch: 0, rc: -1, original: "v2.2.0"},
			},
			want: version{},
		},
		{
			name: "multiple_rcs_across_series",
			tags: []version{
				{major: 2, minor: 1, patch: 0, rc: 0, original: "v2.1.0-rc.0"},
				{major: 2, minor: 2, patch: 0, rc: 3, original: "v2.2.0-rc.3"},
				{major: 2, minor: 2, patch: 0, rc: 1, original: "v2.2.0-rc.1"},
				{major: 2, minor: 1, patch: 0, rc: -1, original: "v2.1.0"},
			},
			want: version{major: 2, minor: 2, patch: 0, rc: 3, original: "v2.2.0-rc.3"},
		},
		{
			name: "single_rc",
			tags: []version{
				{major: 1, minor: 0, patch: 0, rc: 0, original: "v1.0.0-rc.0"},
			},
			want: version{major: 1, minor: 0, patch: 0, rc: 0, original: "v1.0.0-rc.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := findLatestRC(tt.tags)
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_findLatestNonRC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tags []version
		want version
	}{
		{
			name: "empty_list",
			tags: nil,
			want: version{},
		},
		{
			name: "no_non_rcs",
			tags: []version{
				{major: 2, minor: 1, patch: 0, rc: 0, original: "v2.1.0-rc.0"},
				{major: 2, minor: 2, patch: 0, rc: 3, original: "v2.2.0-rc.3"},
			},
			want: version{},
		},
		{
			name: "multiple_releases",
			tags: []version{
				{major: 2, minor: 1, patch: 0, rc: -1, original: "v2.1.0"},
				{major: 2, minor: 2, patch: 0, rc: -1, original: "v2.2.0"},
				{major: 2, minor: 2, patch: 0, rc: 3, original: "v2.2.0-rc.3"},
				{major: 2, minor: 1, patch: 1, rc: -1, original: "v2.1.1"},
			},
			want: version{major: 2, minor: 2, patch: 0, rc: -1, original: "v2.2.0"},
		},
		{
			name: "single_release",
			tags: []version{
				{major: 1, minor: 0, patch: 0, rc: -1, original: "v1.0.0"},
			},
			want: version{major: 1, minor: 0, patch: 0, rc: -1, original: "v1.0.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := findLatestNonRC(tt.tags)
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_findPreviousTag(t *testing.T) {
	t.Parallel()

	tags := []version{
		{major: 2, minor: 1, patch: 0, rc: -1, original: "v2.1.0"},
		{major: 2, minor: 2, patch: 0, rc: 0, original: "v2.2.0-rc.0"},
		{major: 2, minor: 2, patch: 0, rc: 1, original: "v2.2.0-rc.1"},
		{major: 2, minor: 2, patch: 0, rc: -1, original: "v2.2.0"},
	}

	tests := []struct {
		name   string
		newVer version
		want   string
	}{
		{
			name:   "normal_case",
			newVer: version{major: 2, minor: 2, patch: 0, rc: 2, original: "v2.2.0-rc.2"},
			want:   "v2.2.0-rc.1",
		},
		{
			name:   "no_previous",
			newVer: version{major: 1, minor: 0, patch: 0, rc: 0, original: "v1.0.0-rc.0"},
			want:   "",
		},
		{
			name:   "exact_match_excluded",
			newVer: version{major: 2, minor: 2, patch: 0, rc: -1, original: "v2.2.0"},
			want:   "v2.2.0-rc.1",
		},
		{
			name:   "picks_highest_lesser",
			newVer: version{major: 3, minor: 0, patch: 0, rc: -1, original: "v3.0.0"},
			want:   "v2.2.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := findPreviousTag(tags, tt.newVer)
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_filterTagsForSeries(t *testing.T) {
	t.Parallel()

	tags := []version{
		{major: 2, minor: 1, patch: 0, rc: -1, original: "v2.1.0"},
		{major: 2, minor: 2, patch: 0, rc: 0, original: "v2.2.0-rc.0"},
		{major: 2, minor: 2, patch: 0, rc: -1, original: "v2.2.0"},
		{major: 3, minor: 2, patch: 0, rc: -1, original: "v3.2.0"},
	}

	tests := []struct {
		name       string
		major      int
		minor      int
		wantCount  int
		wantFirst  string
		wantSecond string
	}{
		{
			name:       "matching_tags",
			major:      2,
			minor:      2,
			wantCount:  2,
			wantFirst:  "v2.2.0-rc.0",
			wantSecond: "v2.2.0",
		},
		{
			name:      "no_matching_tags",
			major:     4,
			minor:     0,
			wantCount: 0,
		},
		{
			name:      "single_match",
			major:     2,
			minor:     1,
			wantCount: 1,
			wantFirst: "v2.1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := filterTagsForSeries(tags, tt.major, tt.minor)
			require.Len(t, got, tt.wantCount)
			if tt.wantCount > 0 {
				require.Equal(t, tt.wantFirst, got[0].original)
			}
			if tt.wantCount > 1 {
				require.Equal(t, tt.wantSecond, got[1].original)
			}
		})
	}
}

func Test_isStable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		major int
		minor int
		tags  []version
		want  bool
	}{
		{
			name:  "latest_is_minor_plus_one_stable",
			major: 2,
			minor: 20,
			tags: []version{
				{major: 2, minor: 21, patch: 0, rc: -1, original: "v2.21.0"},
			},
			want: true,
		},
		{
			name:  "latest_is_same_minor_not_stable",
			major: 2,
			minor: 21,
			tags: []version{
				{major: 2, minor: 21, patch: 0, rc: -1, original: "v2.21.0"},
			},
			want: false,
		},
		{
			name:  "latest_is_minor_plus_two_not_stable",
			major: 2,
			minor: 19,
			tags: []version{
				{major: 2, minor: 21, patch: 0, rc: -1, original: "v2.21.0"},
			},
			want: false,
		},
		{
			name:  "no_tags",
			major: 2,
			minor: 20,
			tags:  nil,
			want:  false,
		},
		{
			name:  "only_rcs_no_releases",
			major: 2,
			minor: 20,
			tags: []version{
				{major: 2, minor: 21, patch: 0, rc: 0, original: "v2.21.0-rc.0"},
			},
			want: false,
		},
		{
			name:  "different_major_not_stable",
			major: 2,
			minor: 20,
			tags: []version{
				{major: 3, minor: 21, patch: 0, rc: -1, original: "v3.21.0"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, isStable(tt.major, tt.minor, tt.tags))
		})
	}
}

func Test_isHexSHA(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		s    string
		want bool
	}{
		{
			name: "valid_short_sha",
			s:    "abc1234",
			want: true,
		},
		{
			name: "valid_long_sha",
			s:    "abc1234def5678901234567890abcdef12345678",
			want: true,
		},
		{
			name: "valid_uppercase",
			s:    "ABCDEF1234567",
			want: true,
		},
		{
			name: "too_short",
			s:    "abc12",
			want: false,
		},
		{
			name: "exactly_six_chars",
			s:    "abc123",
			want: false,
		},
		{
			name: "non_hex_chars",
			s:    "xyz1234",
			want: false,
		},
		{
			name: "empty",
			s:    "",
			want: false,
		},
		{
			name: "seven_chars_valid",
			s:    "abcdef1",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, isHexSHA(tt.s))
		})
	}
}

func Test_resolveCommit(t *testing.T) {
	t.Parallel()

	t.Run("ShortSHAResolvedToFull", func(t *testing.T) {
		t.Parallel()
		const full = "1234567890abcdef1234567890abcdef12345678"
		var gotArgs []string
		mock := &mockExecutor{
			RunOutputFunc: func(_ string, args ...string) (string, error) {
				gotArgs = args
				return full, nil
			},
		}
		got, err := resolveCommit(mock, "main", "1234567")
		require.NoError(t, err)
		require.Equal(t, full, got)
		require.Equal(t, []string{"rev-parse", "--verify", "1234567^{commit}"}, gotArgs)
	})

	t.Run("EmptyResolvesRefHead", func(t *testing.T) {
		t.Parallel()
		const full = "abcdef1234567890abcdef1234567890abcdef12"
		var gotArgs []string
		mock := &mockExecutor{
			RunOutputFunc: func(_ string, args ...string) (string, error) {
				gotArgs = args
				return full, nil
			},
		}
		got, err := resolveCommit(mock, "main", "")
		require.NoError(t, err)
		require.Equal(t, full, got)
		require.Equal(t, []string{"rev-parse", "origin/main"}, gotArgs)
	})

	t.Run("InvalidSHANotResolved", func(t *testing.T) {
		t.Parallel()
		called := false
		mock := &mockExecutor{
			RunOutputFunc: func(_ string, _ ...string) (string, error) {
				called = true
				return "", nil
			},
		}
		_, err := resolveCommit(mock, "main", "zzzzzzz")
		require.Error(t, err)
		require.False(t, called, "git should not be invoked for an invalid SHA")
	})
}
