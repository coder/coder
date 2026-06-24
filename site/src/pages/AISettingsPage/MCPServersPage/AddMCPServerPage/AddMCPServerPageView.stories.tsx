import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { MockMCPServerConfig } from "#/testHelpers/chatEntities";
import AddMCPServerPageView from "./AddMCPServerPageView";

const meta: Meta<typeof AddMCPServerPageView> = {
	title: "pages/AISettingsPage/MCPServersPage/AddMCPServerPageView",
	component: AddMCPServerPageView,
	args: {
		isSaving: false,
		onCreateServer: fn(
			async (req: TypesGen.CreateMCPServerConfigRequest) =>
				({ ...MockMCPServerConfig, ...req }) as TypesGen.MCPServerConfig,
		),
		onCancel: fn(),
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers/add" },
			routing: { path: "/ai/settings/mcp-servers/add" },
		}),
	},
};

export default meta;
type Story = StoryObj<typeof AddMCPServerPageView>;

export const Default: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const addButton = canvas.getByRole("button", { name: "Add server" });

		await expect(addButton).toBeDisabled();
		await userEvent.type(canvas.getByLabelText(/display name/i), "GitHub");
		await expect(canvas.getByLabelText(/^slug/i)).toHaveValue("github");
		await userEvent.type(
			canvas.getByLabelText(/server url/i),
			"https://api.githubcopilot.com/mcp/",
		);
		await expect(addButton).toBeEnabled();

		await userEvent.click(
			canvas.getByRole("button", { name: /authentication/i }),
		);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			canvas.getByRole("combobox", { name: /authentication method/i }),
		);
		await userEvent.click(body.getByRole("option", { name: "OAuth2" }));
		await expect(canvas.getByLabelText(/client id/i)).toBeInTheDocument();

		await userEvent.click(addButton);
		await waitFor(() => {
			expect(args.onCreateServer).toHaveBeenCalledWith(
				expect.objectContaining({
					display_name: "GitHub",
					slug: "github",
					url: "https://api.githubcopilot.com/mcp/",
					auth_type: "oauth2",
				}),
			);
		});
	},
};
