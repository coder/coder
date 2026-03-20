import { useAuthenticated } from "hooks";
import { useDashboard } from "modules/dashboard/useDashboard";

export const useAIBridgeFeature = () => {
	const { permissions } = useAuthenticated();
	const { entitlements } = useDashboard();

	const entitlement = entitlements.features.aibridge.entitlement;
	const isEntitled =
		entitlement === "entitled" || entitlement === "grace_period";
	const isEnabled = entitlements.features.aibridge.enabled;
	const hasPermission = permissions.viewAnyAIBridgeInterception;

	return { isEntitled, isEnabled, hasPermission };
};
