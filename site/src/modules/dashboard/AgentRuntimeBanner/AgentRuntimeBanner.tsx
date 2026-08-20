import type { FC } from "react";
import type { Feature } from "#/api/typesGenerated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { LicenseBannerView } from "../LicenseBanner/LicenseBannerView";

// Derives the member-facing runtime hours message from the feature's
// serialized thresholds. The backend warning strings target admins and are
// rendered by LicenseBanner behind the deployment permission, so members
// need a message derived client-side. The guard and ladder mirror the
// admin-facing appendAgentRuntimeHoursWarning
// (enterprise/coderd/license/license.go): a non-positive allocation
// disables the feature rather than exhausting it, and each rung supersedes
// the ones below it so at most one message renders.
export const agentRuntimeBannerMessage = (
	feature: Feature | undefined,
): string | null => {
	if (!feature) {
		return null;
	}

	const { actual, entitlement, limit } = feature;
	if (
		(entitlement !== "entitled" && entitlement !== "grace_period") ||
		limit === undefined ||
		limit <= 0 ||
		// Without a measurement there is nothing actionable for members;
		// admins see the measurement diagnostic through LicenseBanner.
		actual === undefined
	) {
		return null;
	}

	// decodeAgentRuntimeHours only sets hard_limit at or above the
	// allocation, so the hard-limit rung is checked first.
	const hardLimit = feature.hard_limit;
	if (hardLimit !== undefined && actual >= hardLimit) {
		return `Your deployment has used ${actual} of the ${limit} Coder Agent runtime hours included in the current license term, reaching the hard limit of ${hardLimit} hours. Contact your deployment administrator.`;
	}
	if (actual >= limit) {
		return `Your deployment has used ${actual} of the ${limit} Coder Agent runtime hours included in the current license term. Contact your deployment administrator.`;
	}
	return null;
};

// The runtime hours banner shown to users without the deployment
// permission; see the mount in DashboardLayout. It reuses the license
// banner chrome, which has no dismiss affordance, so the banner persists
// until the entitlements refetch stops deriving a message.
export const AgentRuntimeBanner: FC = () => {
	const { entitlements } = useDashboard();
	const message = agentRuntimeBannerMessage(
		entitlements.features.agent_runtime_hours,
	);

	if (!message) {
		return null;
	}

	return <LicenseBannerView messages={[{ message, variant: "error" }]} />;
};
