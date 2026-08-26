import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { organizationsPermissions } from "#/api/queries/organizations";
import {
	MockDefaultOrganization,
	MockNoPermissions,
	MockOrganizationPermissions,
	MockPermissions,
	MockUserOwner,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import { AISettingsSidebar } from "./AISettingsSidebar";
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("link", { name: "Models" })).toHaveAttribute(
			"aria-current",
			"page",
		);
	},
};

export const ModelsActiveOnOrganizationRoute: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/ai/settings/organizations/my-organization/models",
			},
			routing: [
				{
					path: "/ai/settings/organizations/:organization/models",
					useStoryElement: true,
				},
			],
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("link", { name: "Models" })).toHaveAttribute(
			"aria-current",
			"page",
		);
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("link", { name: "Models" }),
		).not.toHaveAttribute("aria-current", "page");
	},
};

export const ModelsWithoutDeploymentConfig: Story = {
	args: {
		permissions: {
			...MockNoPermissions,
			editAnyChatModelConfig: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("link", { name: "Models" }),
		).toBeInTheDocument();
		await expect(
			canvas.queryByRole("link", { name: "Coder Agents" }),
		).not.toBeInTheDocument();
	},
};

export const ModelsWithDeletePermissionOnly: Story = {
	args: {
		permissions: {
			...MockNoPermissions,
			deleteAnyChatModelConfig: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("link", { name: "Models" })).toBeVisible();
	},
};

export const ModelsWithReadPermissionOnly: Story = {
	args: {
		permissions: {
			...MockNoPermissions,
			viewAnyChatModelConfig: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("link", { name: "Models" })).toBeVisible();
	},
};

export const OrganizationOnlyRoleCanAccessModels: Story = {
	render: () => <AISettingsSidebar />,
	decorators: [withAuthProvider, withDashboardProvider],
	parameters: {
		user: MockUserOwner,
		permissions: MockNoPermissions,
		organizations: [MockDefaultOrganization],
		queries: [
			{
				key: organizationsPermissions([MockDefaultOrganization.id]).queryKey,
				data: {
					[MockDefaultOrganization.id]: MockOrganizationPermissions,
				},
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("link", { name: "Models" })).toBeVisible();
		await expect(
			canvas.getByRole("link", { name: "Coder Agents" }),
		).toBeVisible();
	},
};

export const NoDeploymentConfig: Story = {
	args: {
		permissions: {
			...MockPermissions,
			editDeploymentConfig: false,
			updateAnyTemplate: false,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByText("Coder Agents")).not.toBeInTheDocument();
		expect(canvas.queryByText("Templates")).not.toBeInTheDocument();
	},
};

export const NoUpdateTemplates: Story = {
	args: {
		permissions: {
			...MockPermissions,
			updateAnyTemplate: false,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("Coder Agents")).toBeVisible();
		expect(canvas.queryByText("Templates")).not.toBeInTheDocument();
		expect(canvas.getByText("Models")).toBeVisible();
	},
};

export const MCPServersForUpdateOnlyAdmin: Story = {
	args: {
		permissions: {
			...MockNoPermissions,
			updateAnyMCPServerConfig: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("link", { name: "MCP servers" }),
		).toBeVisible();
	},
};

export const MCPServersForDeleteOnlyAdmin: Story = {
	args: {
		permissions: {
			...MockNoPermissions,
			deleteAnyMCPServerConfig: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("link", { name: "MCP servers" }),
		).toHaveAttribute("href", "/ai/settings/mcp-servers");
	},
};

export const MCPServersForOrganizationShareOnlyAdmin: Story = {
	args: {
		permissions: MockNoPermissions,
		canShareOrganizationMCPServers: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("link", { name: "MCP servers" }),
		).toHaveAttribute("href", "/ai/settings/mcp-servers");
	},
};

export const MCPServersForCreateOnlyAdmin: Story = {
	args: {
		permissions: {
			...MockNoPermissions,
			createAnyMCPServerConfig: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("link", { name: "MCP servers" }),
		).toHaveAttribute("href", "/ai/settings/mcp-servers/add");
	},
};

export const MCPServersHiddenWithoutPermission: Story = {
	args: {
		permissions: {
			...MockNoPermissions,
			editDeploymentConfig: false,
			viewAnyMCPServerConfigs: false,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.queryByRole("link", { name: "MCP servers" }),
		).not.toBeInTheDocument();
	},
};

export const NoPermissions: Story = {
	args: {
		permissions: MockNoPermissions,
	},
};
