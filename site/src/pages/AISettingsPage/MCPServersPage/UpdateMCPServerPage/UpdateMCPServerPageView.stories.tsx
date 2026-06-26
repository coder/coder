import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { MockCoderMCPServer } from "../testFixtures";
import UpdateMCPServerPageView from "./UpdateMCPServerPageView";

const onUpdateServer = fn(
	async (
		_id: string,
		req: TypesGen.UpdateMCPServerConfigRequest,
	): Promise<unknown> => req,
);

const meta: Meta<typeof UpdateMCPServerPageView> = {
	title: "pages/AISettingsPage/MCPServersPage/UpdateMCPServerPageView",
	component: UpdateMCPServerPageView,
	args: {
		server: MockCoderMCPServer,
		isSaving: false,
		isDeleting: false,
		onUpdateServer,
		onDeleteServer: fn(async () => undefined),
		onToggleEnabled: fn(),
		onCancel: fn(),
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers/mcp-coder" },
			routing: { path: "/ai/settings/mcp-servers/:serverId" },
		}),
	},
};

export default meta;
type Story = StoryObj<typeof UpdateMCPServerPageView>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(canvas.getByLabelText(/display name/i)).toHaveValue("Coder");
		await userEvent.click(
			canvas.getByRole("button", { name: /authentication/i }),
		);
		await expect(canvas.getByLabelText(/client secret/i)).toHaveValue(
			"••••••••••••••••",
		);

		const updateButton = canvas.getByRole("button", { name: "Update server" });
		await expect(updateButton).toBeEnabled();
		await userEvent.click(updateButton);

		await waitFor(() => {
			expect(onUpdateServer).toHaveBeenCalledWith(
				"mcp-coder",
				expect.objectContaining({
					display_name: "Coder",
					slug: "coder",
				}),
			);
		});
		expect(onUpdateServer.mock.calls[0]?.[1]).not.toHaveProperty("enabled");
	},
};
