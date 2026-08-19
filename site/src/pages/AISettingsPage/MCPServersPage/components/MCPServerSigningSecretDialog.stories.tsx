import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
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

export const Default: Story = {
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		await waitFor(() => {
			expect(
				body.getByRole("heading", { name: "Save your MCP signing secret" }),
			).toBeVisible();
		});
		await userEvent.click(body.getByRole("button", { name: "Copy code" }));
		await userEvent.click(body.getByRole("button", { name: "Done" }));
		await expect(args.onClose).toHaveBeenCalledOnce();
	},
};

export const EscapeDoesNotDismissSecret: Story = {
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		await body.findByRole("heading", {
			name: "Save your MCP signing secret",
		});
		await userEvent.keyboard("{Escape}");
		await waitFor(() => {
			expect(
				body.getByRole("heading", {
					name: "Save your MCP signing secret",
				}),
			).toBeVisible();
		});
		await expect(args.onClose).not.toHaveBeenCalled();
	},
};
