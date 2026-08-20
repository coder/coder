import type { FC } from "react";
import type { Feature } from "#/api/typesGenerated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { LicenseBannerView } from "../LicenseBanner/LicenseBannerView";

// The backend warning strings are admin-only (rendered by LicenseBanner
// behind the deployment permission), so the member message is derived
// client-side. Keep the guard and ladder in sync with
// appendAgentRuntimeHoursWarning (enterprise/coderd/license/license.go).
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

// Intentionally non-dismissible: the banner persists until an entitlements
// refetch stops deriving a message.
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
