package focus

import "testing"

// TestEscapeCSVCell is a white-box test of the pure escaping helper itself;
// TestWriteCSV_EscapesRiskyColumnsOnly (csv_test.go) covers it end to end
// through the public WriteCSV API.
//
//nolint:testpackage // exercises the unexported escapeCSVCell directly.
func TestEscapeCSVCell(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"normal", "normal"},
		{"=SUM(A1:A2)", "'=SUM(A1:A2)"},
		{"+1", "'+1"},
		{"-1", "'-1"},
		{"@cmd", "'@cmd"},
		{"\ttab", "'\ttab"},
		{"\rcr", "'\rcr"},
	}
	for _, tc := range cases {
		if got := escapeCSVCell(tc.in); got != tc.want {
			t.Errorf("escapeCSVCell(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
