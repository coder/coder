package filefinder

import (
	"testing"
)

func TestNormalizeQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty",
			input: "",
			want:  "",
		},
		{
			name:  "leading and trailing spaces",
			input: "  hello  ",
			want:  "hello",
		},
		{
			name:  "multiple internal spaces",
			input: "foo   bar   baz",
			want:  "foo bar baz",
		},
		{
			name:  "uppercase to lower",
			input: "FooBar",
			want:  "foobar",
		},
		{
			name:  "backslash to slash",
			input: `foo\bar\baz`,
			want:  "foo/bar/baz",
		},
		{
			name:  "mixed case and spaces",
			input: "  Hello   World  ",
			want:  "hello world",
		},
		{
			name:  "unicode passthrough",
			input: "héllo wörld",
			want:  "héllo wörld",
		},
		{
			name:  "only spaces",
			input: "     ",
			want:  "",
		},
		{
			name:  "single char",
			input: "A",
			want:  "a",
		},
		{
			name:  "slashes preserved",
			input: "/foo/bar/",
			want:  "/foo/bar/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeQuery(tt.input)
			if got != tt.want {
				t.Errorf("normalizeQuery(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractTrigrams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []uint32
	}{
		{
			name:  "too short",
			input: "ab",
			want:  nil,
		},
		{
			name:  "exactly three bytes",
			input: "abc",
			want:  []uint32{uint32('a')<<16 | uint32('b')<<8 | uint32('c')},
		},
		{
			name:  "case insensitive",
			input: "ABC",
			// Should produce the same trigram as "abc".
			want: []uint32{uint32('a')<<16 | uint32('b')<<8 | uint32('c')},
		},
		{
			name:  "deduplication",
			input: "aaaa",
			// "aaa" appears twice but should be deduplicated.
			want: []uint32{uint32('a')<<16 | uint32('a')<<8 | uint32('a')},
		},
		{
			name:  "four bytes produces two trigrams",
			input: "abcd",
			want: []uint32{
				uint32('a')<<16 | uint32('b')<<8 | uint32('c'),
				uint32('b')<<16 | uint32('c')<<8 | uint32('d'),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractTrigrams([]byte(tt.input))
			if !uint32SlicesEqual(got, tt.want) {
				t.Errorf("extractTrigrams(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractBasename(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		wantOff  int
		wantName string
	}{
		{
			name:     "full path",
			path:     "/foo/bar/baz.go",
			wantOff:  9,
			wantName: "baz.go",
		},
		{
			name:     "bare filename",
			path:     "baz.go",
			wantOff:  0,
			wantName: "baz.go",
		},
		{
			name:     "trailing slash",
			path:     "/a/b/",
			wantOff:  3,
			wantName: "b",
		},
		{
			name:     "root slash",
			path:     "/",
			wantOff:  0,
			wantName: "",
		},
		{
			name:     "empty",
			path:     "",
			wantOff:  0,
			wantName: "",
		},
		{
			name:     "single dir with slash",
			path:     "/foo",
			wantOff:  1,
			wantName: "foo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			off, length := extractBasename([]byte(tt.path))
			if off != tt.wantOff {
				t.Errorf("extractBasename(%q) offset = %d, want %d", tt.path, off, tt.wantOff)
			}
			gotName := string([]byte(tt.path)[off : off+length])
			if gotName != tt.wantName {
				t.Errorf("extractBasename(%q) name = %q, want %q", tt.path, gotName, tt.wantName)
			}
		})
	}
}

func TestExtractSegments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		path  string
		want  []string
	}{
		{
			name: "absolute path",
			path: "/foo/bar/baz",
			want: []string{"foo", "bar", "baz"},
		},
		{
			name: "relative path",
			path: "foo/bar",
			want: []string{"foo", "bar"},
		},
		{
			name: "trailing slash",
			path: "/a/b/",
			want: []string{"a", "b"},
		},
		{
			name: "multiple slashes",
			path: "//a///b//",
			want: []string{"a", "b"},
		},
		{
			name: "empty",
			path: "",
			want: nil,
		},
		{
			name: "single segment",
			path: "foo",
			want: []string{"foo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractSegments([]byte(tt.path))
			if len(got) != len(tt.want) {
				t.Fatalf("extractSegments(%q) got %d segments, want %d", tt.path, len(got), len(tt.want))
			}
			for i := range got {
				if string(got[i]) != tt.want[i] {
					t.Errorf("extractSegments(%q)[%d] = %q, want %q", tt.path, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestPrefix1(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want byte
	}{
		{"lowercase", "foo", 'f'},
		{"uppercase", "Foo", 'f'},
		{"empty", "", 0},
		{"digit", "1abc", '1'},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := prefix1([]byte(tt.in))
			if got != tt.want {
				t.Errorf("prefix1(%q) = %d (%c), want %d (%c)", tt.in, got, got, tt.want, tt.want)
			}
		})
	}
}

func TestPrefix2(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want uint16
	}{
		{
			name: "two chars",
			in:   "ab",
			want: uint16('a')<<8 | uint16('b'),
		},
		{
			name: "uppercase",
			in:   "AB",
			want: uint16('a')<<8 | uint16('b'),
		},
		{
			name: "single char",
			in:   "A",
			want: uint16('a') << 8,
		},
		{
			name: "empty",
			in:   "",
			want: 0,
		},
		{
			name: "longer string",
			in:   "Hello",
			want: uint16('h')<<8 | uint16('e'),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := prefix2([]byte(tt.in))
			if got != tt.want {
				t.Errorf("prefix2(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizePathBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "backslash to slash",
			input: `C:\Users\test`,
			want:  "c:/users/test",
		},
		{
			name:  "collapse slashes",
			input: "//foo///bar//",
			want:  "/foo/bar/",
		},
		{
			name:  "lowercase",
			input: "FooBar",
			want:  "foobar",
		},
		{
			name:  "empty",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Make a copy since normalizePathBytes works in-place.
			buf := []byte(tt.input)
			got := string(normalizePathBytes(buf))
			if got != tt.want {
				t.Errorf("normalizePathBytes(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// uint32SlicesEqual reports whether two uint32 slices are equal.
// Nil and empty slices are treated as equal.
func uint32SlicesEqual(a, b []uint32) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
