import type { SettingsNavigationSection } from "#/components/SettingsNavigation/SettingsNavigation";

type UserSettingsNavigationOptions = {
	showSchedulePage: boolean;
	showOAuth2Page: boolean;
};

export const userSettingsNavigation = ({
	showSchedulePage,
	showOAuth2Page,
}: UserSettingsNavigationOptions): readonly SettingsNavigationSection[] => [
	{
		id: "general",
		label: "General",
		items: [
			{
				id: "account",
				label: "Account",
				href: "/settings/account",
				end: true,
			},
			{
				id: "appearance",
				label: "Appearance",
				href: "/settings/appearance",
				end: true,
			},
			{
				id: "notifications",
				label: "Notifications",
				href: "/settings/notifications",
				end: true,
			},
			...(showSchedulePage
				? [
						{
							id: "schedule",
							label: "Schedule",
							href: "/settings/schedule",
							end: true,
						},
					]
				: []),
			{
				id: "security",
				label: "Security",
				href: "/settings/security",
				end: true,
			},
		],
	},
	{
		id: "connected-accounts",
		label: "Connected accounts",
		items: [
			{
				id: "external-auth",
				label: "External authentication",
				href: "/settings/external-auth",
				end: true,
			},
			...(showOAuth2Page
				? [
						{
							id: "oauth2-provider",
							label: "OAuth2 applications",
							href: "/settings/oauth2-provider",
							end: true,
						},
					]
				: []),
		],
	},
	{
		id: "credentials",
		label: "Credentials",
		items: [
			{
				id: "ssh-keys",
				label: "SSH keys",
				href: "/settings/ssh-keys",
				end: true,
			},
			{
				id: "secrets",
				label: "Secrets",
				href: "/settings/secrets",
				end: true,
			},
			{
				id: "tokens",
				label: "Tokens",
				href: "/settings/tokens",
				matchPatterns: ["/settings/tokens", "/settings/tokens/*"],
			},
		],
	},
];
