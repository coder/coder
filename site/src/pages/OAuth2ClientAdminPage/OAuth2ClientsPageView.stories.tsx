import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	type OAuth2ClientSummary,
	OAuth2ClientsPageView,
} from "./OAuth2ClientsPageView";

const clients: OAuth2ClientSummary[] = [
	{
		id: "1",
		name: "Internal deploy bot",
		type: "confidential",
		callbackUrl: "https://deploy.internal.example.com/oauth/callback",
	},
	{
		id: "2",
		name: "Coder CLI",
		type: "public",
		callbackUrl: "http://localhost:4321/callback",
	},
	{
		id: "3",
		name: "Claude Desktop",
		type: "public",
		callbackUrl: "https://claude.ai/api/mcp/auth_callback",
	},
	{
		id: "4",
		name: "Backstage plugin",
		type: "confidential",
		callbackUrl: "https://backstage.example.com/api/auth/coder/handler/frame",
	},
];

const meta: Meta<typeof OAuth2ClientsPageView> = {
	title: "pages/OAuth2ClientAdmin/List",
	component: OAuth2ClientsPageView,
	args: {
		clients,
		onCreate: () => {},
		onSelect: () => {},
	},
};

export default meta;
type Story = StoryObj<typeof OAuth2ClientsPageView>;

/** Type is its own column, so an admin can see at a glance which clients have secrets. */
export const Default: Story = {};

export const Empty: Story = {
	args: {
		clients: [],
	},
};

export const Loading: Story = {
	args: {
		clients: undefined,
		isLoading: true,
	},
};

export const ViewOnly: Story = {
	args: {
		canCreate: false,
	},
};
