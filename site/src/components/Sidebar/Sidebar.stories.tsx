import type { Meta, StoryObj } from "@storybook/react-vite";
import { BotIcon, LockIcon, SettingsIcon, UserLockIcon } from "lucide-react";
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
	render: () => (
		<Sidebar>
			<SidebarHeader
				avatar={<Avatar fallback="Jon" />}
				title="Jon"
				subtitle="jon@coder.com"
			/>
			<div className="flex flex-col gap-1">
				<SidebarGroup
					icon={SettingsIcon}
					label="General"
					href="/settings/account"
				>
					<SidebarNavItem end href="/settings/account">
						Account
					</SidebarNavItem>
					<SidebarNavItem href="/settings/appearance">
						Appearance
					</SidebarNavItem>
					<SidebarNavItem href="/settings/schedule">Schedule</SidebarNavItem>
					<SidebarNavItem href="/settings/notifications">
						Notifications
					</SidebarNavItem>
				</SidebarGroup>
				<SidebarGroup
					icon={UserLockIcon}
					label="Authentication"
					href="/settings/security"
				>
					<SidebarNavItem end href="/settings/security">
						Password
					</SidebarNavItem>
					<SidebarNavItem href="/settings/external-auth">
						External authentication
					</SidebarNavItem>
					<SidebarNavItem href="/settings/oauth2-provider">
						OAuth2 applications
					</SidebarNavItem>
					<SidebarNavItem href="/settings/ssh-keys">SSH keys</SidebarNavItem>
				</SidebarGroup>
				<SidebarGroup icon={LockIcon} label="Security" href="/settings/tokens">
					<SidebarNavItem end href="/settings/tokens">
						Tokens
					</SidebarNavItem>
					<SidebarNavItem href="/settings/secrets">Secrets</SidebarNavItem>
				</SidebarGroup>
			</div>
		</Sidebar>
	),
	parameters: {
		reactRouter: {
			location: {
				path: "/settings/account",
			},
			routing: [
				{ path: "/settings/account", useStoryElement: true },
				{ path: "/settings/appearance", useStoryElement: true },
				{ path: "/settings/schedule", useStoryElement: true },
				{ path: "/settings/notifications", useStoryElement: true },
				{ path: "/settings/external-auth", useStoryElement: true },
				{ path: "/settings/oauth2-provider", useStoryElement: true },
				{ path: "/settings/security", useStoryElement: true },
				{ path: "/settings/ssh-keys", useStoryElement: true },
				{ path: "/settings/tokens", useStoryElement: true },
				{ path: "/settings/secrets", useStoryElement: true },
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
