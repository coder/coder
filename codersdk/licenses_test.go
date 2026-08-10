package codersdk_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

// TestLicenseAgentRuntimeHoursWarningTexts pins the warning-text prefix
// couplings consumed by the dashboard's LicenseBanner
// (site/src/modules/dashboard/LicenseBanner).
func TestLicenseAgentRuntimeHoursWarningTexts(t *testing.T) {
	t.Parallel()

	// Cut rather than Split so a template losing its placeholder fails the
	// test instead of silently turning the whole message into the "prefix".
	templatePrefix := func(text, placeholder string) string {
		t.Helper()
		prefix, _, ok := strings.Cut(text, placeholder)
		require.True(t, ok, "template %q must contain placeholder %q", text, placeholder)
		return prefix
	}

	aiGovNearLimitPrefix := templatePrefix(codersdk.LicenseAIGovernance90PercentWarningText, "%d%%")
	aiGovOverLimitPrefix := templatePrefix(codersdk.LicenseAIGovernanceOverLimitWarningText, "%d")
	softLimitPrefix := templatePrefix(codersdk.LicenseAgentRuntimeHoursSoftLimitWarningText, "%d")

	runtimeTexts := map[string]string{
		"SoftLimit":         codersdk.LicenseAgentRuntimeHoursSoftLimitWarningText,
		"AllocationReached": codersdk.LicenseAgentRuntimeHoursAllocationReachedWarningText,
	}
	for name, text := range runtimeTexts {
		// isMutedWarning renders near-limit matches muted, and
		// isAIGovernanceWarning matches either AI Governance prefix to
		// suppress the banner's client-side over-limit fallback.
		require.False(t, strings.HasPrefix(text, aiGovNearLimitPrefix),
			"%s warning must not share the AI Governance near-limit prefix %q", name, aiGovNearLimitPrefix)
		require.False(t, strings.HasPrefix(text, aiGovOverLimitPrefix),
			"%s warning must not share the AI Governance over-limit prefix %q", name, aiGovOverLimitPrefix)
	}

	// isMutedWarning renders soft-limit matches muted and messageLink drops
	// their sales link, so the allocation-reached warning must not match.
	allocationReachedText := codersdk.LicenseAgentRuntimeHoursAllocationReachedWarningText
	require.False(t, strings.HasPrefix(allocationReachedText, softLimitPrefix),
		"the soft-limit prefix must not classify the allocation-reached warning")
}
