import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { MockDefaultOrganization, MockUserOwner } from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import AddMCPServerPage from "./AddMCPServerPage/AddMCPServerPage";
import MCPServersPage from "./MCPServersPage";
import { MockCoderMCPServer } from "./testFixtures";
import UpdateMCPServerPage from "./UpdateMCPServerPage/UpdateMCPServerPage";

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

export const UpdateLoadsDefaultOrganization: Story = {
	render: () => <UpdateMCPServerPage />,
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers/mcp-coder" },
			routing: { path: "/ai/settings/mcp-servers/:serverId" },
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
		await expect(canvas.getByLabelText(/display name/i)).toHaveValue("Coder");
	},
};
