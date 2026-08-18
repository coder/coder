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
		listPath: "/ai/settings/mcp-servers",
		isSaving: false,
		canSelectUserOIDC: true,
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

export const UserOIDCRequiresDeploymentPermission: Story = {
	args: {
		canSelectUserOIDC: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			canvas.getByRole("button", { name: /authentication/i }),
		);
		await userEvent.click(
			canvas.getByRole("combobox", { name: /authentication method/i }),
		);
		await expect(
			body.getByRole("option", { name: "OAuth2" }),
		).toBeInTheDocument();
		expect(
			body.queryByRole("option", { name: "User OIDC identity" }),
		).not.toBeInTheDocument();
		await expect(canvas.getByLabelText(/display name/i)).toBeEnabled();
	},
};

export const DeleteOnly: Story = {
	args: {
		onUpdateServer: undefined,
		onToggleEnabled: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await expect(canvas.getByLabelText(/display name/i)).toBeDisabled();
		await expect(canvas.getByLabelText(/^slug/i)).toBeDisabled();
		await expect(canvas.getByLabelText(/server url/i)).toBeDisabled();
		await expect(
			canvas.getByRole("button", { name: "Update server" }),
		).toBeDisabled();
		const serverActions = canvas.getByRole("button", {
			name: "Server actions",
		});
		const enabledSwitch = canvas.getByRole("switch", {
			name: "Server enabled",
		});
		await expect(enabledSwitch).toHaveAttribute("aria-disabled", "true");
		await expect(enabledSwitch).toHaveAccessibleDescription(
			"You do not have permission to update this server.",
		);
		serverActions.focus();
		await userEvent.tab();
		await expect(enabledSwitch).toHaveFocus();
		await expect(await body.findByRole("tooltip")).toHaveTextContent(
			"You do not have permission to update this server.",
		);
		await userEvent.click(serverActions);
		await expect(
			await body.findByRole("menuitem", { name: "Remove" }),
		).toBeEnabled();
	},
};
