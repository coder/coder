package main

import (
	"testing"
)

func TestParseVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		ok    bool
		want  version
	}{
		{"v2.32.0", true, version{2, 32, 0, ""}},
		{"v1.0.0", true, version{1, 0, 0, ""}},
		{"v2.32.0-rc.0", true, version{2, 32, 0, "rc.0"}},
		{"v2.32.0-rc.1", true, version{2, 32, 0, "rc.1"}},
		{"v2.32.1-beta.3", true, version{2, 32, 1, "beta.3"}},
		{"2.32.0", false, version{}},
		{"v2.32", false, version{}},
		{"vx.y.z", false, version{}},
		{"", false, version{}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got, ok := parseVersion(tt.input)
			if ok != tt.ok {
				t.Fatalf("parseVersion(%q) ok = %v, want %v", tt.input, ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Fatalf("parseVersion(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestVersionString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		v    version
		want string
	}{
		{version{2, 32, 0, ""}, "v2.32.0"},
		{version{2, 32, 0, "rc.0"}, "v2.32.0-rc.0"},
		{version{1, 0, 0, "beta.1"}, "v1.0.0-beta.1"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			if got := tt.v.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVersionIsRC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		v    version
		want bool
	}{
		{version{2, 32, 0, "rc.0"}, true},
		{version{2, 32, 0, "rc.1"}, true},
		{version{2, 32, 0, ""}, false},
		{version{2, 32, 0, "beta.1"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.v.String(), func(t *testing.T) {
			t.Parallel()
			if got := tt.v.IsRC(); got != tt.want {
				t.Fatalf("IsRC() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVersionRCNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		v    version
		want int
	}{
		{version{2, 32, 0, "rc.0"}, 0},
		{version{2, 32, 0, "rc.5"}, 5},
		{version{2, 32, 0, ""}, -1},
		{version{2, 32, 0, "beta.1"}, -1},
	}

	for _, tt := range tests {
		t.Run(tt.v.String(), func(t *testing.T) {
			t.Parallel()
			if got := tt.v.rcNumber(); got != tt.want {
				t.Fatalf("rcNumber() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestVersionGreaterThan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b version
		want bool
	}{
		// Standard comparisons.
		{version{2, 32, 1, ""}, version{2, 32, 0, ""}, true},
		{version{2, 32, 0, ""}, version{2, 32, 1, ""}, false},
		{version{2, 33, 0, ""}, version{2, 32, 0, ""}, true},
		{version{3, 0, 0, ""}, version{2, 99, 99, ""}, true},

		// Release > RC with same base version.
		{version{2, 32, 0, ""}, version{2, 32, 0, "rc.0"}, true},
		{version{2, 32, 0, "rc.0"}, version{2, 32, 0, ""}, false},

		// RC ordering.
		{version{2, 32, 0, "rc.1"}, version{2, 32, 0, "rc.0"}, true},
		{version{2, 32, 0, "rc.0"}, version{2, 32, 0, "rc.1"}, false},
		{version{2, 32, 0, "rc.10"}, version{2, 32, 0, "rc.9"}, true},
		{version{2, 32, 0, "rc.9"}, version{2, 32, 0, "rc.10"}, false},

		// Equal.
		{version{2, 32, 0, ""}, version{2, 32, 0, ""}, false},
		{version{2, 32, 0, "rc.0"}, version{2, 32, 0, "rc.0"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.a.String()+"_gt_"+tt.b.String(), func(t *testing.T) {
			t.Parallel()
			if got := tt.a.GreaterThan(tt.b); got != tt.want {
				t.Fatalf("%s.GreaterThan(%s) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestVersionEqual(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b version
		want bool
	}{
		{version{2, 32, 0, ""}, version{2, 32, 0, ""}, true},
		{version{2, 32, 0, "rc.0"}, version{2, 32, 0, "rc.0"}, true},
		{version{2, 32, 0, ""}, version{2, 32, 0, "rc.0"}, false},
		{version{2, 32, 0, "rc.0"}, version{2, 32, 0, "rc.1"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.a.String()+"_eq_"+tt.b.String(), func(t *testing.T) {
			t.Parallel()
			if got := tt.a.Equal(tt.b); got != tt.want {
				t.Fatalf("%s.Equal(%s) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
