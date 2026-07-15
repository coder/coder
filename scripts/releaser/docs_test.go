package main

import "testing"

// TestUpdateCalendarLatestReleaseVersionPrefix checks the formatting of the
// "Latest Release" cell produced by updateCalendar. version.String() already
// includes a leading "v", so the link label must not add a second one, while
// the release tag URL keeps the "v" because tags are prefixed with it.
func TestUpdateCalendarLatestReleaseVersionPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		newVer  version
		channel string
		want    string
	}{
		{
			name:    "patch release",
			newVer:  version{Major: 2, Minor: 35, Patch: 1},
			channel: "mainline",
			want:    "[v2.35.1](https://github.com/coder/coder/releases/tag/v2.35.1)",
		},
		{
			name:    "minor release",
			newVer:  version{Major: 2, Minor: 35, Patch: 0},
			channel: "mainline",
			want:    "[v2.35.0](https://github.com/coder/coder/releases/tag/v2.35.0)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rows := []calendarRow{{
				ReleaseName:   "2.35",
				Major:         2,
				Minor:         35,
				ReleaseDate:   "February 03, 2026",
				Status:        "Mainline",
				LatestRelease: "N/A",
			}}

			got := updateCalendar(rows, tt.newVer, tt.channel)

			var cell string
			for _, r := range got {
				if r.Major == tt.newVer.Major && r.Minor == tt.newVer.Minor {
					cell = r.LatestRelease
					break
				}
			}

			if cell != tt.want {
				t.Fatalf("LatestRelease = %q, want %q", cell, tt.want)
			}
		})
	}
}
