package units

import "testing"

func TestRoundTrip(t *testing.T) {
	for _, s := range []string{"512B", "1.5KiB", "10.0MiB", "2.0GiB"} {
		n, err := ParseSize(s)
		if err != nil {
			t.Fatalf("ParseSize(%q): %v", s, err)
		}
		if got := FormatSize(n); got != s && got != s[:len(s)-3]+".0"+s[len(s)-3:] {
			t.Errorf("FormatSize(ParseSize(%q)) = %q", s, got)
		}
	}
}
