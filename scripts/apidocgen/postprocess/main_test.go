package main

import (
	"bufio"
	"strings"
	"testing"
)

func TestExtractSectionName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		section         string
		want            string
		wantErrContains []string
	}{
		{
			name:    "FrontMatterTitle",
			section: "---\ntitle: Workspaces\n---\n\n## Get workspace\n",
			want:    "Workspaces",
		},
		{
			name:    "FrontMatterTitleWithSpaces",
			section: "---\ntitle: Workspace Proxies\n---\n",
			want:    "Workspace Proxies",
		},
		{
			name:    "FrontMatterTitleDoubleQuoted",
			section: "---\ntitle: \"General\"\n---\n",
			want:    "General",
		},
		{
			name:    "FrontMatterTitleSingleQuoted",
			section: "---\ntitle: 'Enterprise'\n---\n",
			want:    "Enterprise",
		},
		{
			name:    "FrontMatterLeadingBlankLines",
			section: "\n\n---\ntitle: Agents\n---\n",
			want:    "Agents",
		},
		{
			name:    "FrontMatterTitleNotFirstKey",
			section: "---\ndescription: ignored\ntitle: Templates\n---\n",
			want:    "Templates",
		},
		{
			name:    "LegacyHeadingFallback",
			section: "# Members\n\nSome body text.\n",
			want:    "Members",
		},
		{
			name:    "LegacyHeadingLeadingBlankLines",
			section: "\n\n# Schemas\n",
			want:    "Schemas",
		},
		{
			name:    "FrontMatterMissingTitle",
			section: "---\ndescription: no title here\n---\n",
			wantErrContains: []string{
				`no non-empty "title:" key`,
				"no title here",
			},
		},
		{
			name:    "FrontMatterEmptyTitle",
			section: "---\ntitle:\n---\n",
			wantErrContains: []string{
				`no non-empty "title:" key`,
				"section starts:",
			},
		},
		{
			name:    "NoHeadingOrFrontMatter",
			section: "Just some text\nwith no heading\n",
			wantErrContains: []string{
				"section header not found",
				"Just some text",
			},
		},
		{
			name:            "Empty",
			section:         "",
			wantErrContains: []string{"section header not found"},
		},
		{
			// A line past bufio.Scanner's token limit makes Scan return false
			// with the reason only in Err(); surface it rather than mislabeling
			// it a missing header.
			name:            "ScannerError",
			section:         strings.Repeat("a", bufio.MaxScanTokenSize+1),
			wantErrContains: []string{"scanning section"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := extractSectionName([]byte(tt.section))
			if len(tt.wantErrContains) > 0 {
				if err == nil {
					t.Fatalf("extractSectionName(%q): expected error, got nil (result %q)", tt.section, got)
				}
				for _, want := range tt.wantErrContains {
					if !strings.Contains(err.Error(), want) {
						t.Fatalf("extractSectionName(%q): error %q does not contain %q", tt.section, err.Error(), want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("extractSectionName(%q): unexpected error: %v", tt.section, err)
			}
			if got != tt.want {
				t.Fatalf("extractSectionName(%q) = %q, want %q", tt.section, got, tt.want)
			}
		})
	}
}

func TestSectionPreview(t *testing.T) {
	t.Parallel()

	// Whitespace, including newlines, collapses to single spaces.
	if got, want := sectionPreview([]byte("---\ndescription:  x\n---\n")), "--- description: x ---"; got != want {
		t.Fatalf("sectionPreview() = %q, want %q", got, want)
	}

	// Content past the rune cap is truncated and marked.
	if got, want := sectionPreview([]byte(strings.Repeat("a", 200))), strings.Repeat("a", 120)+"..."; got != want {
		t.Fatalf("sectionPreview() long = %q, want %q", got, want)
	}
}
