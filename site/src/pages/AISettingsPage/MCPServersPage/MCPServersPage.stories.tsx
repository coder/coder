import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockDefaultOrganization,
	MockOrganization2,
	MockUserOwner,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import AddMCPServerPage from "./AddMCPServerPage/AddMCPServerPage";
import MCPServersPage from "./MCPServersPage";
import { MockCoderMCPServer } from "./testFixtures";
import UpdateMCPServerPage from "./UpdateMCPServerPage/UpdateMCPServerPage";

const MockOrganization2MCPServer: TypesGen.MCPServerConfig = {
	...MockCoderMCPServer,
	id: "mcp-org2",
	display_name: "Org2 Search",
	slug: "org2-search",
	organization_id: MockOrganization2.id,
};

const meta = {
	title: "pages/AISettingsPage/MCPServersPage/MCPServersPage",
	component: MCPServersPage,
	decorators: [withAuthProvider, withDashboardProvider],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		permissions: { editDeploymentConfig: true },
		organizations: [MockDefaultOrganization],
	},
} satisfies Meta<typeof MCPServersPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const ListUsesDefaultOrganization: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers" },
			routing: { path: "/ai/settings/mcp-servers" },
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([
			MockCoderMCPServer,
		]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(API.experimental.getMCPServerConfigs).toHaveBeenCalledWith(
				MockDefaultOrganization.id,
			);
		});
		await expect(canvas.getByText("Coder")).toBeVisible();
		expect(
			canvas.queryByRole("button", { name: "Organization" }),
		).not.toBeInTheDocument();
	},
};

export const ListSwitchesOrganization: Story = {
	parameters: {
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers" },
			routing: { path: "/ai/settings/mcp-servers" },
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getMCPServerConfigs").mockImplementation(
			async (organization) =>
				organization === MockOrganization2.id
					? [MockOrganization2MCPServer]
					: [MockCoderMCPServer],
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByText("Coder")).toBeVisible();
		await userEvent.click(canvas.getByRole("button", { name: "Organization" }));
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("option", {
				name: MockOrganization2.display_name,
			}),
		);
		await expect(await canvas.findByText("Org2 Search")).toBeVisible();
		expect(canvas.queryByText("Coder")).not.toBeInTheDocument();
		expect(API.experimental.getMCPServerConfigs).toHaveBeenCalledWith(
			MockOrganization2.id,
		);
	},
};

export const AddUsesDefaultOrganization: Story = {
	render: () => <AddMCPServerPage />,
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers/add" },
			routing: { path: "/ai/settings/mcp-servers/add" },
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "createMCPServerConfig").mockResolvedValue(
			MockCoderMCPServer,
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(canvas.getByLabelText(/display name/i), "GitHub");
		await userEvent.type(
			canvas.getByLabelText(/server url/i),
			"https://api.githubcopilot.com/mcp/",
		);
		await userEvent.click(canvas.getByRole("button", { name: "Add server" }));
		await waitFor(() => {
			expect(API.experimental.createMCPServerConfig).toHaveBeenCalledWith(
				MockDefaultOrganization.id,
				expect.objectContaining({
					display_name: "GitHub",
					slug: "github",
					url: "https://api.githubcopilot.com/mcp/",
				}),
			);
		});
	},
};

export const AddToSelectedOrganization: Story = {
	render: () => <AddMCPServerPage />,
	parameters: {
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({
			location: {
				path: "/ai/settings/mcp-servers/add",
				searchParams: { org: MockOrganization2.name },
			},
			routing: { path: "/ai/settings/mcp-servers/add" },
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "createMCPServerConfig").mockResolvedValue(
			MockOrganization2MCPServer,
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("button", { name: "Organization" }),
		).toHaveTextContent(MockOrganization2.display_name);
		await userEvent.type(canvas.getByLabelText(/display name/i), "GitHub");
		await userEvent.type(
			canvas.getByLabelText(/server url/i),
			"https://api.githubcopilot.com/mcp/",
		);
		await userEvent.click(canvas.getByRole("button", { name: "Add server" }));
		await waitFor(() => {
			expect(API.experimental.createMCPServerConfig).toHaveBeenCalledWith(
				MockOrganization2.id,
				expect.objectContaining({ display_name: "GitHub" }),
			);
		});
	},
};

export const AddSwitchesOrganization: Story = {
	render: () => <AddMCPServerPage />,
	parameters: {
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers/add" },
			routing: { path: "/ai/settings/mcp-servers/add" },
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "createMCPServerConfig").mockResolvedValue(
			MockOrganization2MCPServer,
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(canvas.getByLabelText(/display name/i), "GitHub");
		await userEvent.type(
			canvas.getByLabelText(/server url/i),
			"https://api.githubcopilot.com/mcp/",
		);
		await userEvent.click(canvas.getByRole("button", { name: "Organization" }));
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("option", {
				name: MockOrganization2.display_name,
			}),
		);
		await expect(canvas.getByLabelText(/display name/i)).toHaveValue("GitHub");
		await expect(
			canvas.getByRole("link", { name: /back to mcp servers/i }),
		).toHaveAttribute(
			"href",
			`/ai/settings/mcp-servers?org=${MockOrganization2.name}`,
		);
		await userEvent.click(canvas.getByRole("button", { name: "Add server" }));
		await waitFor(() => {
			expect(API.experimental.createMCPServerConfig).toHaveBeenCalledWith(
				MockOrganization2.id,
				expect.objectContaining({ display_name: "GitHub" }),
			);
		});
	},
};

export const UpdateShowsDetailLoadError: Story = {
	render: () => <UpdateMCPServerPage />,
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers/mcp-coder" },
			routing: { path: "/ai/settings/mcp-servers/:serverId" },
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getMCPServerConfig").mockRejectedValue(
			new Error("Failed to load MCP server."),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText("Failed to load MCP server."),
		).toBeVisible();
		expect(canvas.queryByLabelText(/display name/i)).not.toBeInTheDocument();
	},
};

export const UpdateLoadsServerById: Story = {
	render: () => <UpdateMCPServerPage />,
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers/mcp-coder" },
			routing: { path: "/ai/settings/mcp-servers/:serverId" },
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getMCPServerConfig").mockResolvedValue(
			MockCoderMCPServer,
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(API.experimental.getMCPServerConfig).toHaveBeenCalledWith(
				MockCoderMCPServer.id,
			);
		});
		await expect(canvas.getByLabelText(/display name/i)).toHaveValue("Coder");
	},
};
