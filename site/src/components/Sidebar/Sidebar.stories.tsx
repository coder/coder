import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	BotIcon,
	CalendarCogIcon,
	FingerprintIcon,
	KeyIcon,
	LockIcon,
	SettingsIcon,
	UserIcon,
} from "lucide-react";
import { Outlet } from "react-router";
import { Avatar } from "#/components/Avatar/Avatar";
import {
	Sidebar,
	SidebarGroup,
	SidebarHeader,
	SidebarNavItem,
} from "./Sidebar";

const meta: Meta<typeof Sidebar> = {
	title: "components/Sidebar",
	component: Sidebar,
};

export default meta;
type Story = StoryObj<typeof Sidebar>;

export const Default: Story = {
	decorators: [
		(Story) => {
			return (
				<div className="flex gap-2">
					<Story />
					<Outlet />
				</div>
			);
		},
	],
	render: () => (
		<Sidebar>
			<SidebarHeader
				avatar={<Avatar fallback="Jon" />}
				title="Jon"
				subtitle="jon@coder.com"
			/>
			<SidebarNavItem href="account" icon={UserIcon}>
				Account
			</SidebarNavItem>
			<SidebarNavItem href="schedule" icon={CalendarCogIcon}>
				Schedule
			</SidebarNavItem>
			<SidebarNavItem href="security" icon={LockIcon}>
				Security
			</SidebarNavItem>
			<SidebarNavItem href="ssh-keys" icon={FingerprintIcon}>
				SSH Keys
			</SidebarNavItem>
			<SidebarNavItem href="tokens" icon={KeyIcon}>
				Tokens
			</SidebarNavItem>
		</Sidebar>
	),
	parameters: {
		reactRouter: {
			location: {
				path: "/account",
			},
			routing: [
				{
					path: "/",
					useStoryElement: true,
					children: [
						{
							path: "account",
							element: <>Account page</>,
						},
						{
							path: "schedule",
							element: <>Schedule page</>,
						},
						{
							path: "security",
							element: <>Security page</>,
						},
						{
							path: "ssh-keys",
							element: <>SSH Keys</>,
						},
						{
							path: "tokens",
							element: <>Tokens page</>,
						},
					],
				},
			],
		},
	},
};

export const WithGroups: Story = {
	render: () => (
		<Sidebar>
			<SidebarGroup
				icon={SettingsIcon}
				label="General"
				href="/deployment/overview"
			>
				<SidebarNavItem href="/deployment/overview">Overview</SidebarNavItem>
				<SidebarNavItem href="/deployment/licenses">Licenses</SidebarNavItem>
			</SidebarGroup>
			<SidebarGroup
				icon={BotIcon}
				label="Coder Agents"
				href="/ai/settings/coder-agents"
			>
				<SidebarNavItem end href="/ai/settings/coder-agents">
					Overview
				</SidebarNavItem>
				<SidebarNavItem href="/ai/settings/models">Models</SidebarNavItem>
				<SidebarNavItem href="/ai/settings/mcp-servers">
					MCP servers
				</SidebarNavItem>
			</SidebarGroup>
		</Sidebar>
	),
	parameters: {
		reactRouter: {
			location: { path: "/ai/settings/models" },
			routing: [
				{ path: "/deployment/overview", useStoryElement: true },
				{ path: "/deployment/licenses", useStoryElement: true },
				{ path: "/ai/settings/coder-agents", useStoryElement: true },
				{ path: "/ai/settings/models", useStoryElement: true },
				{ path: "/ai/settings/mcp-servers", useStoryElement: true },
			],
		},
	},
};
