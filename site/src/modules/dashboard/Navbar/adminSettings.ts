import { linkToAuditing } from "#/modules/navigation";

/**
 * Permissions that determine which items appear in the Admin settings menu.
 * Shared by the desktop `DeploymentDropdown` and the mobile `MobileMenu` so
 * both surfaces render the same set of items from a single source of truth.
 */
export type AdminSettingsPermissions = {
	canViewDeployment: boolean;
	canViewOrganizations: boolean;
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
 * permissions. Organizations is always available; the rest are gated behind
 * their respective permissions.
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
	{ label: "Organizations", to: "/organizations" },
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
 * menu. Organizations alone does not gate visibility, matching prior behavior.
 */
export const canViewAdminSettings = (
	permissions: AdminSettingsPermissions,
): boolean => Object.values(permissions).some((canView) => canView);
