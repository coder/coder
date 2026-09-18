import { useQuery } from "react-query";
import { aiSpendOrganizations } from "#/api/queries/aiBridge";
import { useDashboard } from "#/modules/dashboard/useDashboard";

type UseCanViewAISpendOptions = {
	enabled?: boolean;
};

// Top-level navigation must admit organization group member readers without
// site-wide AI settings permissions. The organization query keeps its cached
// result once disabled, so the answer follows the entitlement itself.
export const useCanViewAISpend = (options: UseCanViewAISpendOptions = {}) => {
	const { entitlements } = useDashboard();
	const isEnabled = entitlements.features.aibridge.enabled;
	const organizationsQuery = useQuery({
		...aiSpendOrganizations(),
		enabled: isEnabled && (options.enabled ?? true),
	});
	return {
		canView: isEnabled && (organizationsQuery.data?.length ?? 0) > 0,
		isLoading: organizationsQuery.isLoading,
		error: organizationsQuery.error,
	};
};
