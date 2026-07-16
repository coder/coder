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

type AdminSettingsSection = {
	/** Optional heading rendered above the section's items. */
	label?: string;
	items: AdminSettingsItem[];
};

/**
 * Builds the ordered, sectioned list of Admin settings menu items for the
 * given permissions. Organizations is always available; the rest are gated
 * behind their respective permissions. Sections with no visible items are
 * omitted.
 */
export const getAdminSettingsSections = ({
	canViewDeployment,
	canViewAuditLog,
	canViewConnectionLog,
	canViewAIBridge,
	canViewAISettings,
	canViewHealth,
}: AdminSettingsPermissions): AdminSettingsSection[] => {
	const sections: AdminSettingsSection[] = [
		{
			items: [
				...(canViewDeployment
					? [{ label: "Deployment", to: "/deployment" }]
					: []),
				{ label: "Organizations", to: "/organizations" },
				...(canViewAISettings ? [{ label: "AI", to: "/ai/settings" }] : []),
			],
		},
		{
			label: "Logs",
			items: [
				...(canViewAuditLog
					? [{ label: "Audit logs", to: linkToAuditing }]
					: []),
				...(canViewConnectionLog
					? [{ label: "Connection logs", to: "/connectionlog" }]
					: []),
				...(canViewAIBridge
					? [{ label: "AI session logs", to: "/ai-gateway/sessions" }]
					: []),
			],
		},
		{
			items: canViewHealth ? [{ label: "Healthcheck", to: "/health" }] : [],
		},
	];
	return sections.filter((section) => section.items.length > 0);
};

/**
 * Whether the user has any permission that should surface the Admin settings
 * menu. Organizations alone does not gate visibility, matching prior behavior.
 */
export const canViewAdminSettings = (
	permissions: AdminSettingsPermissions,
): boolean => Object.values(permissions).some((canView) => canView);
