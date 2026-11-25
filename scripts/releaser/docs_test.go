package main //nolint:testpackage // Tests unexported release helpers.

import (
	"strings"
	"testing"
)

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

// TestUpdateCalendarNotReleasedRowName checks that a "Not Released" row is
// promoted to "Mainline" with a major.minor "Release name" link (patch
// omitted) once its minor version is released.
func TestUpdateCalendarNotReleasedRowName(t *testing.T) {
	t.Parallel()

	rows := []calendarRow{{
		ReleaseName:   "2.36",
		Major:         2,
		Minor:         36,
		Status:        "Not Released",
		LatestRelease: "N/A",
	}}

	got := updateCalendar(rows, version{Major: 2, Minor: 36, Patch: 0}, "mainline")

	if got[0].Status != "Mainline" {
		t.Errorf("Status = %q, want %q", got[0].Status, "Mainline")
	}
	const want = "[2.36](https://coder.com/changelog/coder-2-36)"
	if got[0].ReleaseName != want {
		t.Fatalf("ReleaseName = %q, want %q", got[0].ReleaseName, want)
	}
}

// TestRenderCalendarTableSeparatorWidth ensures every separator segment is as
// wide as its header segment, matching what markdown-table-formatter enforces
// in CI. The first column was previously rendered one dash short, which made
// `make fmt` fail on wizard-generated docs PRs.
func TestRenderCalendarTableSeparatorWidth(t *testing.T) {
	t.Parallel()

	rows := []calendarRow{
		{
			ReleaseName:   "[2.39](https://coder.com/changelog/coder-2-39)",
			Major:         2,
			Minor:         39,
			ReleaseDate:   "November 03, 2026",
			Status:        "Mainline",
			LatestRelease: "[v2.39.2](https://github.com/coder/coder/releases/tag/v2.39.2)",
		},
		{
			ReleaseName:   "2.40",
			Major:         2,
			Minor:         40,
			Status:        "Not Released",
			LatestRelease: "N/A",
		},
	}

	lines := strings.Split(strings.TrimSuffix(renderCalendarTable(rows), "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least header and separator lines, got %d", len(lines))
	}
	header := strings.Split(lines[0], "|")
	sep := strings.Split(lines[1], "|")
	if len(header) != len(sep) {
		t.Fatalf("header has %d columns, separator has %d", len(header), len(sep))
	}
	for i := range header {
		if len(header[i]) != len(sep[i]) {
			t.Errorf("column %d: header width %d, separator width %d", i, len(header[i]), len(sep[i]))
		}
	}
}
