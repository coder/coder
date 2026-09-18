import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import { MCPServerSigningSecretDialog } from "./MCPServerSigningSecretDialog";

const meta: Meta<typeof MCPServerSigningSecretDialog> = {
	title: "pages/AISettingsPage/MCPServersPage/MCPServerSigningSecretDialog",
	component: MCPServerSigningSecretDialog,
	args: {
		secret: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		onClose: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof MCPServerSigningSecretDialog>;

export const Default: Story = {};

export const EscapeDoesNotDismissSecret: Story = {
	play: async ({ canvasElement }) => {
		await within(canvasElement.ownerDocument.body).findByRole("heading", {
			name: "Save your MCP signing secret",
		});
		await userEvent.keyboard("{Escape}");
	},
};
