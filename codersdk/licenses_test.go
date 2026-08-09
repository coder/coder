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

	// The banner classifies a warning starting with the soft-limit prefix
	// as advisory: muted variant, no sales link. The allocation-reached
	// warning must not match it, or it would render as an advisory.
	allocationReachedText := codersdk.LicenseAgentRuntimeHoursAllocationReachedWarningText
	require.False(t, strings.HasPrefix(allocationReachedText, softLimitPrefix),
		"the soft-limit prefix must not classify the allocation-reached warning")
}
