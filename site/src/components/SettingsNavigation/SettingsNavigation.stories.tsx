import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { Avatar } from "#/components/Avatar/Avatar";
import { setupMatchMedia } from "#/testHelpers/matchMedia";
import { belowLgViewportMediaQuery } from "#/utils/mobile";
import { SettingsNavigation } from "./SettingsNavigation";
import type { SettingsNavigationSection } from "./types";

const groupedSections: readonly SettingsNavigationSection[] = [
	{
		id: "general",
		label: "General",
		items: [
			{ id: "account", label: "Account", href: "/account", end: true },
			{
				id: "appearance",
				label: "Appearance",
				href: "/appearance",
				end: true,
			},
			{
				id: "notifications",
				label: "Notifications",
				href: "/notifications",
				end: true,
			},
			{ id: "schedule", label: "Schedule", href: "/schedule", end: true },
			{ id: "security", label: "Security", href: "/security", end: true },
		],
	},
	{
		id: "connected-accounts",
		label: "Connected accounts",
		items: [
			{
				id: "external-auth",
				label: "External authentication",
				href: "/external-auth",
				end: true,
			},
			{
				id: "oauth2",
				label: "OAuth2 applications",
				href: "/oauth2",
				end: true,
			},
		],
	},
	{
		id: "credentials",
		label: "Credentials",
		items: [
			{ id: "ssh", label: "SSH keys", href: "/ssh-keys", end: true },
			{ id: "secrets", label: "Secrets", href: "/secrets", end: true },
			{ id: "tokens", label: "Tokens", href: "/tokens", end: true },
		],
	},
];

const flatSections: readonly SettingsNavigationSection[] = [
	{
		id: "workspace",
		items: [
			{ id: "general", label: "General", href: "/workspace", end: true },
			{
				id: "parameters",
				label: "Parameters",
				href: "/workspace/parameters",
				end: true,
			},
			{
				id: "schedule",
				label: "Schedule",
				href: "/workspace/schedule",
				end: true,
			},
			{
				id: "sharing",
				label: "Sharing",
				href: "/workspace/sharing",
				end: true,
			},
		],
	},
];

const meta: Meta<typeof SettingsNavigation> = {
	title: "components/SettingsNavigation",
	component: SettingsNavigation,
	parameters: {
		layout: "fullscreen",
	},
	beforeEach: () => {
		localStorage.clear();
	},
};

export default meta;
type Story = StoryObj<typeof SettingsNavigation>;

const content = (
	<div className="max-w-3xl">
		<h1 className="text-2xl font-semibold text-content-primary">Account</h1>
		<p className="text-content-secondary">
			Update the settings for this page. The content column scrolls
			independently from the navigation.
		</p>
		<div className="mt-8 h-[900px] rounded border border-solid border-border bg-surface-secondary" />
	</div>
);

export const GroupedExpanded: Story = {
	args: {
		title: "User settings",
		sections: groupedSections,
		storageKey: "settings-navigation-story-grouped-expanded",
		children: content,
		pageAdornment: <Avatar size="sm" fallback="Ada" />,
	},
	parameters: {
		reactRouter: {
			location: { path: "/account" },
			routing: { path: "*", useStoryElement: true },
		},
	},
};

export const GroupedCollapsedMenu: Story = {
	...GroupedExpanded,
	args: {
		...GroupedExpanded.args,
		storageKey: "settings-navigation-story-grouped-collapsed",
	},
	beforeEach: () => {
		localStorage.setItem("settings-navigation-story-grouped-collapsed", "true");
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Account" }),
		);
	},
};

export const FlatExpanded: Story = {
	args: {
		title: "Workspace settings",
		sections: flatSections,
		storageKey: "settings-navigation-story-flat-expanded",
		children: content,
	},
	parameters: {
		reactRouter: {
			location: { path: "/workspace" },
			routing: { path: "*", useStoryElement: true },
		},
	},
};

export const WithDashboardBanners: Story = {
	args: GroupedExpanded.args,
	render: (args) => (
		<div data-dashboard-layout className="flex min-h-full flex-col">
			<div className="flex h-10 shrink-0 items-center justify-center bg-surface-warning text-content-primary">
				License banner
			</div>
			<div className="flex h-8 shrink-0 items-center justify-center bg-surface-info text-content-primary">
				Announcement banner
			</div>
			<div
				data-dashboard-body
				className="flex min-h-screen flex-col justify-between"
			>
				<div className="h-[72px] shrink-0 border-0 border-b border-solid border-border" />
				<main className="relative flex min-h-0 flex-1 flex-col">
					<SettingsNavigation {...args} />
				</main>
				<div className="flex h-9 shrink-0 items-center border-0 border-t border-solid border-border px-3 text-xs">
					Deployment information
				</div>
			</div>
		</div>
	),
	parameters: GroupedExpanded.parameters,
};

export const NarrowCollapsed: Story = {
	...GroupedExpanded,
	beforeEach: () => {
		localStorage.clear();
		return setupMatchMedia({ [belowLgViewportMediaQuery]: true }).restore;
	},
	parameters: {
		...GroupedExpanded.parameters,
		viewport: { defaultViewport: "ipad" },
	},
};
