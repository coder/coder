import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { mockCoderMCPServer } from "../testFixtures";
import UpdateMCPServerPageView from "./UpdateMCPServerPageView";

const meta: Meta<typeof UpdateMCPServerPageView> = {
	title: "pages/AISettingsPage/MCPServersPage/UpdateMCPServerPageView",
	component: UpdateMCPServerPageView,
	args: {
		server: mockCoderMCPServer,
		isSaving: false,
		isDeleting: false,
		onUpdateServer: fn(async () => undefined),
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

export const Default: Story = {};
