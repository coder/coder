import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { ExternalAuthSettingsPageView } from "./ExternalAuthSettingsPageView";

const meta: Meta<typeof ExternalAuthSettingsPageView> = {
	title: "pages/DeploymentSettingsPage/ExternalAuthSettingsPageView",
	component: ExternalAuthSettingsPageView,
	args: {
		config: {
			external_auth: [
				{
					id: "0000-1111",
					type: "GitHub",
					client_id: "client_id",
					regex: "regex",
					api_base_url: "",
					auth_url: "",
					token_url: "",
					validate_url: "",
					redirect_url: "",
					revoke_url: "",
					app_install_url: "https://github.com/apps/coder/installations/new",
					app_installations_url: "",
					no_refresh: false,
					scopes: [],
					device_flow: true,
					device_code_url: "",
					display_icon: "",
					display_name: "GitHub",
					mcp_url: "",
					mcp_tool_allow_regex: "",
					mcp_tool_deny_regex: "",
					code_challenge_methods_supported: ["S256"],
				},
			],
		},
		isEntitled: false,
		canViewPremium: true,
	},
};

export default meta;
type Story = StoryObj<typeof ExternalAuthSettingsPageView>;

export const Page: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByText("Connect multiple Git and OAuth providers at once."),
		).toBeVisible();
		await expect(
			canvas.getByText("Match providers by regex per host"),
		).toBeVisible();
		await expect(
			canvas.getByRole("link", { name: "Start trial for free" }),
		).toHaveAttribute("href", "/deployment/premium");
	},
};

export const Entitled: Story = {
	args: {
		isEntitled: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.queryByText("Connect multiple Git and OAuth providers at once."),
		).not.toBeInTheDocument();
	},
};

export const PaywallWithoutLicenseAccess: Story = {
	args: {
		canViewPremium: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByText(/contact your deployment administrator/i),
		).toBeVisible();
		await expect(
			canvas.queryByRole("link", { name: "Start trial for free" }),
		).not.toBeInTheDocument();
	},
};

export const Empty: Story = {
	args: {
		config: {
			external_auth: [],
		},
	},
};
