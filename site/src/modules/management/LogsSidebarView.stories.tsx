import type { Meta, StoryObj } from "@storybook/react-vite";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import LogsSidebarView from "./LogsSidebarView";

const meta: Meta<typeof LogsSidebarView> = {
	title: "modules/management/LogsSidebarView",
	component: LogsSidebarView,
	args: {
		canViewAuditLog: true,
		canViewConnectionLog: true,
		canViewAIBridge: true,
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/logs/audit" },
			routing: [
				{ path: "/logs/audit", useStoryElement: true },
				{ path: "/logs/connection", useStoryElement: true },
				{ path: "/logs/ai-sessions", useStoryElement: true },
			],
		}),
	},
};

export default meta;
type Story = StoryObj<typeof LogsSidebarView>;

export const AuditActive: Story = {};

export const ConnectionActive: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/logs/connection" },
			routing: [{ path: "/logs/connection", useStoryElement: true }],
		}),
	},
};

export const AISessionsActive: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/logs/ai-sessions" },
			routing: [{ path: "/logs/ai-sessions", useStoryElement: true }],
		}),
	},
};

export const AuditOnly: Story = {
	args: {
		canViewConnectionLog: false,
		canViewAIBridge: false,
	},
};

export const NoPermissions: Story = {
	args: {
		canViewAuditLog: false,
		canViewConnectionLog: false,
		canViewAIBridge: false,
	},
};
