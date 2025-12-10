import type { Meta, StoryObj } from "@storybook/react-vite";
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
					auth_url: "",
					token_url: "",
					validate_url: "",
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
					code_challenge_methods_supported: ["S256"]
				},
			],
		},
	},
};

export default meta;
type Story = StoryObj<typeof ExternalAuthSettingsPageView>;

export const Page: Story = {};
