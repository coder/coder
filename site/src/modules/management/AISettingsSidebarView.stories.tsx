import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { MockNoPermissions, MockPermissions } from "#/testHelpers/entities";
import AISettingsSidebarView from "./AISettingsSidebarView";

const meta: Meta<typeof AISettingsSidebarView> = {
	title: "modules/management/AISettingsSidebarView",
	component: AISettingsSidebarView,
	args: {
		permissions: MockPermissions,
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
				{ path: "/ai/settings/instructions", useStoryElement: true },
				{ path: "/ai/settings/lifecycle", useStoryElement: true },
			],
		}),
	},
};

export default meta;
type Story = StoryObj<typeof AISettingsSidebarView>;

export const CoderAgentsActive: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("link", { name: "Models" })).toBeVisible();
		await expect(
			canvas.queryByRole("link", { name: "Spend" }),
		).not.toBeInTheDocument();
	},
};

export const ModelsActive: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/models" },
			routing: [{ path: "/ai/settings/models", useStoryElement: true }],
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

export const ProvidersActive: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/providers" },
			routing: [{ path: "/ai/settings/providers", useStoryElement: true }],
		}),
	},
};

export const MCPServersActive: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers" },
			routing: [{ path: "/ai/settings/mcp-servers", useStoryElement: true }],
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("link", { name: "MCP servers" }),
		).toBeVisible();
	},
};

export const NoDeploymentConfig: Story = {
	args: {
		permissions: {
			...MockPermissions,
			editDeploymentConfig: false,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByText("Coder Agents")).not.toBeInTheDocument();
		expect(canvas.queryByText("MCP servers")).not.toBeInTheDocument();
		expect(canvas.queryByText("Templates")).not.toBeInTheDocument();
	},
};

export const NoUpdateTemplates: Story = {
	args: {
		permissions: {
			...MockPermissions,
			updateTemplates: false,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("Coder Agents")).toBeVisible();
		expect(canvas.queryByText("Templates")).not.toBeInTheDocument();
		expect(canvas.getByText("Models")).toBeVisible();
	},
};

export const NoPermissions: Story = {
	args: {
		permissions: MockNoPermissions,
	},
};
