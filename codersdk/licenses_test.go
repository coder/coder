package codersdk_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

// TestLicenseAgentRuntimeHoursWarningTexts pins the string couplings the
// dashboard's LicenseBanner (site/src/modules/dashboard/LicenseBanner)
// depends on. The banner classifies warnings by prefix: a warning starting
// with the AI Governance near-limit prefix renders in the muted variant and
// suppresses the client-side AI Governance over-limit fallback. It also
// dispatches links by exact template matching, which requires that no
// template be a prefix of another.
func TestLicenseAgentRuntimeHoursWarningTexts(t *testing.T) {
	t.Parallel()

	aiGovNearLimitPrefix := strings.Split(codersdk.LicenseAIGovernance90PercentWarningText, "%d%%")[0]
	aiGovOverLimitPrefix := strings.Split(codersdk.LicenseAIGovernanceOverLimitWarningText, "%d")[0]

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

	require.False(t,
		strings.HasPrefix(
			codersdk.LicenseAgentRuntimeHoursAllocationReachedWarningText,
			codersdk.LicenseAgentRuntimeHoursSoftLimitWarningText,
		),
		"the runtime hours warning templates must not be prefixes of each other")
	require.False(t,
		strings.HasPrefix(
			codersdk.LicenseAgentRuntimeHoursSoftLimitWarningText,
			codersdk.LicenseAgentRuntimeHoursAllocationReachedWarningText,
		),
		"the runtime hours warning templates must not be prefixes of each other")
}
