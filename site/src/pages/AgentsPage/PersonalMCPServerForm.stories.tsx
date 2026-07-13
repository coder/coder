import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { MCPServerForm } from "#/pages/AISettingsPage/MCPServersPage/components/MCPServerForm";
import { MockPersonalMCPServer } from "#/pages/AISettingsPage/MCPServersPage/testFixtures";

const meta: Meta<typeof MCPServerForm> = {
	title: "pages/AgentsPage/PersonalMCPServerForm",
	component: MCPServerForm,
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents/settings/mcp-servers/add" },
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
type Story = StoryObj<typeof MCPServerForm>;

export const Create: Story = {
	args: {
		variant: "personal",
		isSaving: false,
		onCreateServer: fn(
			async (req: TypesGen.CreateMCPServerConfigRequest) =>
				({ ...MockPersonalMCPServer, ...req }) as TypesGen.MCPServerConfig,
		),
		onCancel: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const addButton = canvas.getByRole("button", { name: "Add server" });

		await userEvent.type(canvas.getByLabelText(/display name/i), "Notes");
		await userEvent.type(
			canvas.getByLabelText(/server url/i),
			"https://mcp.example.com/notes",
		);

		// The personal variant hides the deployment availability policy.
		await userEvent.click(canvas.getByRole("button", { name: /behavior/i }));
		await expect(
			canvas.queryByLabelText(/availability/i),
		).not.toBeInTheDocument();
		await expect(canvas.getByLabelText("Model intent")).toBeInTheDocument();

		await userEvent.click(addButton);
		await waitFor(() => {
			expect(args.onCreateServer).toHaveBeenCalledWith(
				expect.objectContaining({
					display_name: "Notes",
					slug: "notes",
					url: "https://mcp.example.com/notes",
					personal: true,
					availability: "default_off",
				}),
			);
		});
	},
};

export const EditWithDelete: Story = {
	args: {
		variant: "personal",
		server: MockPersonalMCPServer,
		isSaving: false,
		isDeleting: false,
		onUpdateServer: fn(async () => undefined),
		onDeleteServer: fn(async () => undefined),
		onToggleEnabled: fn(),
		onCancel: fn(),
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: `/agents/settings/mcp-servers/${MockPersonalMCPServer.id}`,
			},
			routing: [
				{ path: "/agents/settings/mcp-servers", useStoryElement: true },
				{
					path: "/agents/settings/mcp-servers/:serverId",
					useStoryElement: true,
				},
			],
		}),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await expect(canvas.getByLabelText(/display name/i)).toHaveValue("Notes");

		await userEvent.click(
			canvas.getByRole("button", { name: "Server actions" }),
		);
		await userEvent.click(body.getByRole("menuitem", { name: /remove/i }));
		const dialog = await body.findByRole("dialog");
		await userEvent.click(
			within(dialog).getByRole("button", { name: /delete/i }),
		);
		await waitFor(() => {
			expect(args.onDeleteServer).toHaveBeenCalledWith(
				MockPersonalMCPServer.id,
			);
		});
	},
};
