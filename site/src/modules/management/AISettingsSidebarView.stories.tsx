import type { Meta, StoryObj } from "@storybook/react-vite";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { SidebarContext } from "#/components/Sidebar/SidebarContext";
import { MockNoPermissions, MockPermissions } from "#/testHelpers/entities";
import AISettingsSidebarView from "./AISettingsSidebarView";

const meta: Meta<typeof AISettingsSidebarView> = {
	title: "modules/management/AISettingsSidebarView",
	component: AISettingsSidebarView,
	args: {
		permissions: MockPermissions,
		activeSection: "coder-agents",
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/coder-agents" },
			routing: [
				{ path: "/ai/settings/governance", useStoryElement: true },
				{ path: "/ai/settings/gateway-keys", useStoryElement: true },
				{ path: "/ai/settings/providers", useStoryElement: true },
				{ path: "/ai/settings/coder-agents", useStoryElement: true },
				{ path: "/ai/settings/models", useStoryElement: true },
				{ path: "/ai/settings/mcp-servers", useStoryElement: true },
				{ path: "/ai/settings/templates", useStoryElement: true },
				{ path: "/ai/settings/spend", useStoryElement: true },
				{ path: "/ai/settings/instructions", useStoryElement: true },
				{ path: "/ai/settings/lifecycle", useStoryElement: true },
			],
		}),
	},
};

export default meta;
type Story = StoryObj<typeof AISettingsSidebarView>;

export const CoderAgentsActive: Story = {};

export const ModelsActive: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/models" },
			routing: [{ path: "/ai/settings/models", useStoryElement: true }],
		}),
	},
};

export const SpendActive: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/spend" },
			routing: [{ path: "/ai/settings/spend", useStoryElement: true }],
		}),
	},
};

export const LifecycleActive: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/lifecycle" },
			routing: [{ path: "/ai/settings/lifecycle", useStoryElement: true }],
		}),
	},
};

export const GovernanceActive: Story = {
	args: {
		activeSection: "governance",
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/governance" },
			routing: [{ path: "/ai/settings/governance", useStoryElement: true }],
		}),
	},
};

export const GatewayKeysActive: Story = {
	args: {
		activeSection: "gateway-keys",
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/gateway-keys" },
			routing: [{ path: "/ai/settings/gateway-keys", useStoryElement: true }],
		}),
	},
};

export const ProvidersActive: Story = {
	args: {
		activeSection: "providers",
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/providers" },
			routing: [{ path: "/ai/settings/providers", useStoryElement: true }],
		}),
	},
};

export const Collapsed: Story = {
	decorators: [
		(Story) => (
			<SidebarContext.Provider
				value={{
					collapsed: true,
					expand: () => {},
					collapse: () => {},
					toggle: () => {},
				}}
			>
				<Story />
			</SidebarContext.Provider>
		),
	],
};

export const NoDeploymentConfig: Story = {
	args: {
		permissions: {
			...MockPermissions,
			editDeploymentConfig: false,
		},
	},
};

export const NoPermissions: Story = {
	args: {
		permissions: MockNoPermissions,
	},
};
