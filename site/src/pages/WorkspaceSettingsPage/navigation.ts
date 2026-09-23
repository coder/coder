import type { SettingsNavigationSection } from "#/components/SettingsNavigation/SettingsNavigation";

export const workspaceSettingsNavigation = (
	basePath: string,
	showSharing: boolean,
): readonly SettingsNavigationSection[] => [
	{
		id: "workspace",
		items: [
			{
				id: "general",
				label: "General",
				href: basePath,
				end: true,
			},
			{
				id: "parameters",
				label: "Parameters",
				href: `${basePath}/parameters`,
				end: true,
			},
			{
				id: "schedule",
				label: "Schedule",
				href: `${basePath}/schedule`,
				end: true,
			},
			...(showSharing
				? [
						{
							id: "sharing",
							label: "Sharing",
							href: `${basePath}/sharing`,
							end: true,
						},
					]
				: []),
		],
	},
];
