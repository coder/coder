import type { Meta, StoryObj } from "@storybook/react-vite";
import { SidebarContext } from "#/components/Sidebar/SidebarContext";
import LogsSidebarView from "./LogsSidebarView";

const meta: Meta<typeof LogsSidebarView> = {
	title: "modules/management/LogsSidebarView",
	component: LogsSidebarView,
	args: {
		canViewAuditLog: true,
		canViewConnectionLog: true,
		canViewAIBridge: true,
		activeSection: "audit",
	},
};

export default meta;
type Story = StoryObj<typeof LogsSidebarView>;

export const AuditActive: Story = {};

export const ConnectionActive: Story = {
	args: {
		activeSection: "connection",
	},
};

export const AISessionsActive: Story = {
	args: {
		activeSection: "ai-sessions",
	},
};

export const Collapsed: Story = {
	decorators: [
		(Story) => (
			<SidebarContext.Provider
				value={{ collapsed: true, expand: () => {}, toggle: () => {} }}
			>
				<Story />
			</SidebarContext.Provider>
		),
	],
};

export const NoAuditLog: Story = {
	args: {
		canViewAuditLog: false,
	},
};

export const NoConnectionLog: Story = {
	args: {
		canViewConnectionLog: false,
	},
};

export const NoAIBridge: Story = {
	args: {
		canViewAIBridge: false,
	},
};

export const AuditOnly: Story = {
	args: {
		canViewConnectionLog: false,
		canViewAIBridge: false,
	},
};
