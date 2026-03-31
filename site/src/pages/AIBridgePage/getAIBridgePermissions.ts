import type { Entitlements } from "#/api/typesGenerated";
import type { Permissions } from "#/modules/permissions";

// Users are allowed to view their own request logs via the API,
// but an AI Bridge page is only visible if the feature is enabled and
// the user has the `viewAnyAIBridgeInterception` permission. (as it's
// defined in the Admin settings dropdown).
export const getAIBridgePermissions = (
	entitlements: Entitlements,
	permissions: Permissions,
) => {
	// the user is entitled if they have either "entitled" or "grace_period"
	// status for the AI Bridge feature
	const isEntitled =
		entitlements.features.aibridge.entitlement === "entitled" ||
		entitlements.features.aibridge.entitlement === "grace_period";
	// the feature is enabled if it's toggled on in the admin settings
	const isEnabled = entitlements.features.aibridge.enabled;
	// the user has permission if they have the `viewAnyAIBridgeInterception` permission
	const hasPermission = permissions.viewAnyAIBridgeInterception;

	return { isEntitled, isEnabled, hasPermission };
};
