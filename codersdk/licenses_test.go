package codersdk_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

// TestLicenseAgentRuntimeHoursWarningTexts pins the string couplings the
// dashboard's LicenseBanner (site/src/modules/dashboard/LicenseBanner)
// depends on. The banner classifies warnings by the literal prefix before a
// template's first placeholder: a warning starting with the AI Governance
// near-limit prefix or the runtime hours soft-limit prefix renders in the
// muted variant, and the AI Governance near-limit prefix additionally
// suppresses the client-side over-limit fallback. A shared prefix would make
// the classifier conflate two different warnings.
func TestLicenseAgentRuntimeHoursWarningTexts(t *testing.T) {
	t.Parallel()

	// Cut rather than Split so a template losing its placeholder fails the
	// test instead of silently turning the whole message into the "prefix".
	cutPrefix := func(text, placeholder string) string {
		t.Helper()
		prefix, _, ok := strings.Cut(text, placeholder)
		require.True(t, ok, "template %q must contain placeholder %q", text, placeholder)
		return prefix
	}

	aiGovNearLimitPrefix := cutPrefix(codersdk.LicenseAIGovernance90PercentWarningText, "%d%%")
	aiGovOverLimitPrefix := cutPrefix(codersdk.LicenseAIGovernanceOverLimitWarningText, "%d")
	softLimitPrefix := cutPrefix(codersdk.LicenseAgentRuntimeHoursSoftLimitWarningText, "%d")
	allocationPrefix := cutPrefix(codersdk.LicenseAgentRuntimeHoursAllocationReachedWarningText, "%d")

	runtimeTexts := map[string]string{
		"SoftLimit":         codersdk.LicenseAgentRuntimeHoursSoftLimitWarningText,
		"AllocationReached": codersdk.LicenseAgentRuntimeHoursAllocationReachedWarningText,
	}
	for name, text := range runtimeTexts {
		require.False(t, strings.HasPrefix(text, aiGovNearLimitPrefix),
			"%s warning must not share the AI Governance near-limit prefix %q", name, aiGovNearLimitPrefix)
		require.False(t, strings.HasPrefix(text, aiGovOverLimitPrefix),
			"%s warning must not share the AI Governance over-limit prefix %q", name, aiGovOverLimitPrefix)
	}

	// The banner tells the advisory soft-limit warning apart from the
	// allocation-reached warning by these prefixes, so they must differ
	// before the first placeholder.
	require.NotEqual(t, softLimitPrefix, allocationPrefix,
		"the runtime hours warning templates must diverge before their first placeholder")
	require.False(t, strings.HasPrefix(softLimitPrefix, allocationPrefix),
		"the allocation-reached prefix must not classify the soft-limit warning")
	require.False(t, strings.HasPrefix(allocationPrefix, softLimitPrefix),
		"the soft-limit prefix must not classify the allocation-reached warning")
}
