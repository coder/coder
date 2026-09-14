package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDeveloperBuckets pins the set of options offered for the
// "Number of developers" prompt. If this test fails, also update the
// matching list in site/src/pages/SetupPage/SetupPageView.tsx
// (numberOfDevelopersOptions) and coordinate with the licensor service owner,
// since the same string is forwarded to v2-licensor.coder.com/trial.
func TestDeveloperBuckets(t *testing.T) {
	t.Parallel()
	require.Equal(t, []string{
		"1 - 50",
		"51 - 100",
		"101 - 200",
		"201 - 500",
		"501 - 1000",
		"1001 - 2500",
		"2500+",
	}, developerBuckets)
}
