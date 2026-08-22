import type { Meta, StoryObj } from "@storybook/react-vite";
import { OAuth2ConsentRedirectView } from "./OAuth2ConsentRedirectView";

const meta: Meta<typeof OAuth2ConsentRedirectView> = {
	title: "pages/OAuth2ConsentPage/Redirect",
	component: OAuth2ConsentRedirectView,
	args: {
		clientName: "Claude Desktop",
		redirectUri: "https://claude.ai/api/mcp/auth_callback?state=abc123",
	},
};

export default meta;
type Story = StoryObj<typeof OAuth2ConsentRedirectView>;

export const Approved: Story = {
	args: {
		decision: "approved",
	},
};

export const Denied: Story = {
	args: {
		decision: "denied",
	},
};
