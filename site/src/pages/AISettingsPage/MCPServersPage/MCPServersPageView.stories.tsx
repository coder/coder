import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import MCPServersPageView from "./MCPServersPageView";
import {
	MockCoderMCPServer,
	MockGitHubMCPServer,
	MockImageMCPServer,
	MockMemoryMCPServer,
} from "./testFixtures";

const meta: Meta<typeof MCPServersPageView> = {
	title: "pages/AISettingsPage/MCPServersPage/MCPServersPageView",
	component: MCPServersPageView,
	args: {
		isLoading: false,
		error: null,
		servers: [
			MockCoderMCPServer,
			MockGitHubMCPServer,
			MockImageMCPServer,
			MockMemoryMCPServer,
		],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers" },
			routing: [
				{ path: "/ai/settings/mcp-servers", useStoryElement: true },
				{ path: "/ai/settings/mcp-servers/add", useStoryElement: true },
				{ path: "/ai/settings/mcp-servers/:serverId", useStoryElement: true },
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
			canvas.getByRole("button", { name: /add server/i }),
		).toBeInTheDocument();
		await expect(canvas.getByText("Coder")).toBeInTheDocument();
		await expect(canvas.getByText("GitHub")).toBeInTheDocument();
		await expect(canvas.getByText("Image")).toBeInTheDocument();
		await expect(canvas.getByText("API key")).toBeInTheDocument();
		await expect(canvas.getAllByText("Enabled").length).toBeGreaterThan(0);
		await expect(canvas.getByText("Disabled")).toBeInTheDocument();
	},
};

export const Loading: Story = {
	args: {
		isLoading: true,
		servers: [],
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

export const LoadError: Story = {
	args: {
		error: new Error("Failed to load MCP servers"),
		servers: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Failed to load MCP servers")).toBeVisible();
		await expect(
			canvas.queryByText("No MCP servers configured"),
		).not.toBeInTheDocument();
	},
};
