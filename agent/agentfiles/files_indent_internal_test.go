package agentfiles

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Direct unit tests for the indent-splice helpers. These test the
// functions in isolation so a helper bug surfaces here with a
// descriptive failure instead of as a rendered-file mismatch deep
// in an integration test.

func TestDetectIndentUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		lines    []string
		wantUnit string
		wantOK   bool
	}{
		{
			name:     "Empty",
			lines:    nil,
			wantUnit: "",
			wantOK:   false,
		},
		{
			name:     "NoIndent",
			lines:    []string{"foo\n", "bar\n"},
			wantUnit: "",
			wantOK:   false,
		},
		{
			name:     "TabOnly",
			lines:    []string{"\tfoo\n", "\t\tbar\n"},
			wantUnit: "\t",
			wantOK:   true,
		},
		{
			name:     "FourSpaceUniform",
			lines:    []string{"    foo\n", "        bar\n"},
			wantUnit: "    ",
			wantOK:   true,
		},
		{
			name:     "TwoSpaceUniform",
			lines:    []string{"  foo\n", "    bar\n"},
			wantUnit: "  ",
			wantOK:   true,
		},
		{
			name:     "GCDReducesFourAndSixToTwo",
			lines:    []string{"    foo\n", "      bar\n"},
			wantUnit: "  ",
			wantOK:   true,
		},
		{
			name:     "MixedAcrossLinesTabAndSpace",
			lines:    []string{"\tfoo\n", "    bar\n"},
			wantUnit: "",
			wantOK:   false,
		},
		{
			name:     "MixedWithinLeadTabThenSpace",
			lines:    []string{"\t    foo\n"},
			wantUnit: "",
			wantOK:   false,
		},
		{
			name:     "MixedWithinLeadSpaceThenTab",
			lines:    []string{" \tfoo\n"},
			wantUnit: "",
			wantOK:   false,
		},
		{
			// DEREM-33 regression: a 2sp whitespace-only line in
			// a 4sp-indented region must not pull the GCD down.
			name:     "WhitespaceOnlyLineSkipped",
			lines:    []string{"    foo\n", "  \n", "    bar\n"},
			wantUnit: "    ",
			wantOK:   true,
		},
		{
			name:     "OnlyWhitespaceOnlyLines",
			lines:    []string{"  \n", "    \n"},
			wantUnit: "",
			wantOK:   false,
		},
		{
			name:     "BlankLineIgnored",
			lines:    []string{"\n", "    foo\n"},
			wantUnit: "    ",
			wantOK:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotUnit, gotOK := detectIndentUnit(tc.lines)
			require.Equal(t, tc.wantUnit, gotUnit)
			require.Equal(t, tc.wantOK, gotOK)
		})
	}
}

func TestIndentGCD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b int
		want int
	}{
		{"BothZero", 0, 0, 0},
		{"AZero", 0, 4, 4},
		{"BZero", 4, 0, 4},
		{"Equal", 4, 4, 4},
		{"Coprime", 3, 5, 1},
		{"CommonFactorTwo", 4, 6, 2},
		{"CommonFactorFour", 8, 12, 4},
		{"TwoSpaceAndFourSpace", 2, 4, 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, indentGCD(tc.a, tc.b))
		})
	}
}

func TestIndentLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		lead      string
		unit      string
		wantLevel int
		wantOK    bool
	}{
		{
			name:      "EmptyLead",
			lead:      "",
			unit:      "    ",
			wantLevel: 0,
			wantOK:    true,
		},
		{
			name:      "CleanMultipleOne",
			lead:      "    ",
			unit:      "    ",
			wantLevel: 1,
			wantOK:    true,
		},
		{
			name:      "CleanMultipleThreeTwoSp",
			lead:      "      ",
			unit:      "  ",
			wantLevel: 3,
			wantOK:    true,
		},
		{
			name:      "CleanMultipleTwoTab",
			lead:      "\t\t",
			unit:      "\t",
			wantLevel: 2,
			wantOK:    true,
		},
		{
			name:      "NonMultipleLength",
			lead:      "   ",
			unit:      "    ",
			wantLevel: 0,
			wantOK:    false,
		},
		{
			// Even when the length divides evenly, the lead must
			// be composed of repetitions of the unit.
			name:      "LengthDividesButCompositionMismatches",
			lead:      "\t ",
			unit:      "  ",
			wantLevel: 0,
			wantOK:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotLevel, gotOK := indentLevel(tc.lead, tc.unit)
			require.Equal(t, tc.wantLevel, gotLevel)
			require.Equal(t, tc.wantOK, gotOK)
		})
	}
}

func TestTranslateIndentLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		rLead      string
		sLead      string
		cLead      string
		searchUnit string
		fileUnit   string
		want       string
		wantOK     bool
	}{
		{
			// Caller sends a 4sp search; inserted line is 8sp
			// (one level deeper). File uses tabs, matched at
			// 1-tab depth. Expected: 2 tabs.
			name:       "PositiveDeltaWrap",
			rLead:      "        ",
			sLead:      "    ",
			cLead:      "\t",
			searchUnit: "    ",
			fileUnit:   "\t",
			want:       "\t\t",
			wantOK:     true,
		},
		{
			// Inserted line at the same level as its reference.
			name:       "ZeroDeltaSameLevel",
			rLead:      "    ",
			sLead:      "    ",
			cLead:      "\t",
			searchUnit: "    ",
			fileUnit:   "\t",
			want:       "\t",
			wantOK:     true,
		},
		{
			// Inserted line shallower than the reference's
			// level by more than the file_base: target goes
			// negative, helper bails.
			name:       "NegativeDeltaBelowFileBase",
			rLead:      "",
			sLead:      "        ",
			cLead:      "\t",
			searchUnit: "    ",
			fileUnit:   "\t",
			want:       "",
			wantOK:     false,
		},
		{
			// Malformed rLead (3 spaces under a 4sp unit).
			name:       "MalformedRLead",
			rLead:      "   ",
			sLead:      "    ",
			cLead:      "\t",
			searchUnit: "    ",
			fileUnit:   "\t",
			want:       "",
			wantOK:     false,
		},
		{
			// 4sp LLM into a 2sp file at matched-4sp baseline.
			// rep_level=2, search_base=1, file_base=2,
			// target=3, emit "      " (6sp).
			name:       "CrossStyle4spTo2sp",
			rLead:      "        ",
			sLead:      "    ",
			cLead:      "    ",
			searchUnit: "    ",
			fileUnit:   "  ",
			want:       "      ",
			wantOK:     true,
		},
		{
			// 2sp LLM into a tab file.
			// rep_level=2, search_base=1, file_base=1,
			// target=2, emit "\t\t".
			name:       "CrossStyle2spToTab",
			rLead:      "    ",
			sLead:      "  ",
			cLead:      "\t",
			searchUnit: "  ",
			fileUnit:   "\t",
			want:       "\t\t",
			wantOK:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, gotOK := translateIndentLevel(tc.rLead, tc.sLead, tc.cLead, tc.searchUnit, tc.fileUnit)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.wantOK, gotOK)
		})
	}
}
