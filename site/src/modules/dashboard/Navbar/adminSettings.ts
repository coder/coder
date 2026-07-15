import { linkToAuditing } from "#/modules/navigation";

/**
 * Permissions that determine which items appear in the Admin settings menu.
 * Shared by the desktop `DeploymentDropdown` and the mobile `MobileMenu` so
 * both surfaces render the same set of items from a single source of truth.
 */
export type AdminSettingsPermissions = {
	canViewDeployment: boolean;
	canViewAuditLog: boolean;
	canViewConnectionLog: boolean;
	canViewAIBridge: boolean;
	canViewAISettings: boolean;
	canViewHealth: boolean;
};

type AdminSettingsItem = {
	label: string;
	to: string;
};

/**
 * Builds the ordered list of Admin settings menu items for the given
 * permissions. Every item is gated behind its respective permission.
 */
export const getAdminSettingsItems = ({
	canViewDeployment,
	canViewAuditLog,
	canViewConnectionLog,
	canViewAIBridge,
	canViewAISettings,
	canViewHealth,
}: AdminSettingsPermissions): AdminSettingsItem[] => [
	...(canViewDeployment ? [{ label: "Deployment", to: "/deployment" }] : []),
	...(canViewAISettings ? [{ label: "AI", to: "/ai/settings" }] : []),
	...(canViewAuditLog ? [{ label: "Audit logs", to: linkToAuditing }] : []),
	...(canViewConnectionLog
		? [{ label: "Connection logs", to: "/connectionlog" }]
		: []),
	...(canViewAIBridge
		? [{ label: "AI sessions", to: "/ai-gateway/sessions" }]
		: []),
	...(canViewHealth ? [{ label: "Healthcheck", to: "/health" }] : []),
];

/**
 * Whether the user has any permission that should surface the Admin settings
 * menu. Organizations is intentionally excluded: it is not an admin setting,
 * so it lives in the user menu instead and must not surface this menu.
 */
export const canViewAdminSettings = (
	permissions: AdminSettingsPermissions,
): boolean => Object.values(permissions).some((canView) => canView);
