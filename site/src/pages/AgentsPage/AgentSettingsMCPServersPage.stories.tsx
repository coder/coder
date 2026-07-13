import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import MCPServersPageView from "#/pages/AISettingsPage/MCPServersPage/MCPServersPageView";
import { MockPersonalMCPServer } from "#/pages/AISettingsPage/MCPServersPage/testFixtures";

const meta: Meta<typeof MCPServersPageView> = {
	title: "pages/AgentsPage/AgentSettingsMCPServersPage",
	component: MCPServersPageView,
	args: {
		isLoading: false,
		error: null,
		servers: [
			MockPersonalMCPServer,
			{
				...MockPersonalMCPServer,
				id: "mcp-personal-disabled",
				display_name: "Scratchpad",
				slug: "scratchpad",
				enabled: false,
			},
		],
		basePath: "/agents/settings/mcp-servers",
		description:
			"Configure your own MCP servers that provide additional tools for Coder Agents. Only you can use them.",
		variant: "personal",
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents/settings/mcp-servers" },
			routing: [
				{ path: "/agents/settings/mcp-servers", useStoryElement: true },
				{ path: "/agents/settings/mcp-servers/add", useStoryElement: true },
				{
					path: "/agents/settings/mcp-servers/:serverId",
					useStoryElement: true,
				},
			],
		}),
	},
};

export default meta;
type Story = StoryObj<typeof MCPServersPageView>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("heading", { name: "Personal MCP servers" }),
		).toBeInTheDocument();
		await expect(canvas.getByText("Notes")).toBeInTheDocument();
		await expect(canvas.getByText("Scratchpad")).toBeInTheDocument();
		await expect(canvas.getByText("Enabled")).toBeInTheDocument();
		await expect(canvas.getByText("Disabled")).toBeInTheDocument();
		// Personal servers have no deployment availability policy.
		await expect(canvas.queryByText("Availability")).not.toBeInTheDocument();
	},
};

export const Empty: Story = {
	args: {
		servers: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("No MCP servers configured"),
		).toBeInTheDocument();
	},
};

export const Loading: Story = {
	args: {
		isLoading: true,
		servers: [],
	},
};

export const LoadError: Story = {
	args: {
		error: new Error("Failed to load MCP servers"),
		servers: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Failed to load MCP servers")).toBeVisible();
	},
};
