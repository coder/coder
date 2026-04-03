package coderd

import "testing"

func TestNormalizeChangelogAssetPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain filename",
			input: "2.30-hero.webp",
			want:  "2.30-hero.webp",
		},
		{
			name:  "assets prefix",
			input: "assets/2.30-hero.webp",
			want:  "2.30-hero.webp",
		},
		{
			name:  "leading slash and assets prefix",
			input: "/assets/2.30-hero.webp",
			want:  "2.30-hero.webp",
		},
		{
			name:  "whitespace",
			input: "  assets/2.30-hero.webp  ",
			want:  "2.30-hero.webp",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeChangelogAssetPath(tc.input)
			if got != tc.want {
				t.Fatalf("normalizeChangelogAssetPath(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestChangelogImageURL(t *testing.T) {
	t.Parallel()

	got := changelogImageURL("assets/2.30-hero.webp")
	want := "/api/v2/changelog/assets/2.30-hero.webp"
	if got != want {
		t.Fatalf("changelogImageURL() = %q, want %q", got, want)
	}

	if got := changelogImageURL(""); got != "" {
		t.Fatalf("changelogImageURL(\"\") = %q, want empty string", got)
	}
}
