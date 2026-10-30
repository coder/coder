import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { MockDefaultOrganization } from "#/testHelpers/entities";
import AddMCPServerPageView from "./AddMCPServerPageView";

const meta: Meta<typeof AddMCPServerPageView> = {
	title: "pages/AISettingsPage/MCPServersPage/AddMCPServerPageView",
	component: AddMCPServerPageView,
	args: {
		isSaving: false,
		canCreate: true,
		canSelectUserOIDC: true,
		accessURL: "https://coder.example.com",
		canViewServerList: true,
		organizations: [MockDefaultOrganization],
		organization: MockDefaultOrganization,
		onSelectOrganization: fn(),
		onCreateServer: fn(
			async (_req: TypesGen.CreateMCPServerConfigRequest) => true,
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

		await expect(
			canvas.getByLabelText(
				`Organization ${MockDefaultOrganization.display_name}`,
			),
		).toBeVisible();
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
		await userEvent.click(
			canvas.getByRole("combobox", { name: /authentication method/i }),
		);
		await userEvent.click(screen.getByRole("option", { name: "OAuth2" }));
		await expect(canvas.getByLabelText(/client id/i)).toBeInTheDocument();
		await waitFor(() => {
			expect(screen.queryByRole("option", { name: "OAuth2" })).toBeNull();
		});

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
		await expect(canvas.getByLabelText(/display name/i)).toHaveValue("");
		await expect(canvas.getByLabelText(/^slug/i)).toHaveValue("");
		await expect(canvas.getByLabelText(/server url/i)).toHaveValue("");
		await expect(addButton).toBeDisabled();
	},
};

export const APIKeyAuthentication: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /authentication/i }),
		);
		await userEvent.click(
			canvas.getByRole("combobox", { name: /authentication method/i }),
		);
		await userEvent.click(screen.getByRole("option", { name: "API key" }));
	},
};

export const UserOIDCRequiresDeploymentPermission: Story = {
	args: {
		canSelectUserOIDC: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /authentication/i }),
		);
		await userEvent.click(
			canvas.getByRole("combobox", { name: /authentication method/i }),
		);
		await expect(
			screen.getByRole("option", { name: "OAuth2" }),
		).toBeInTheDocument();
		expect(
			screen.queryByRole("option", { name: "User OIDC identity" }),
		).not.toBeInTheDocument();
	},
};

export const SlackPreset: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("radio", { name: /^Slack/ }));
		await userEvent.click(
			canvas.getByRole("switch", {
				name: "Only reply in the originating thread",
			}),
		);
	},
};

export const GitHubPreset: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("radio", { name: /^GitHub/ }));
		await userEvent.click(canvas.getByRole("checkbox", { name: "Actions" }));
	},
};

/** Switching presets after typing asks before it replaces the details. */
export const SwitchPresetAfterTyping: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(canvas.getByLabelText(/display name/i), "Mine");
		await userEvent.click(canvas.getByRole("radio", { name: /^Slack/ }));
		await screen.findByRole("dialog");
	},
};

export const UserTokenAuthentication: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /authentication/i }),
		);
		await userEvent.click(
			canvas.getByRole("combobox", { name: /authentication method/i }),
		);
		await userEvent.click(
			screen.getByRole("option", { name: "Per-user token" }),
		);
	},
};
