import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { TextPreviewDialog } from "./TextPreviewDialog";

const meta: Meta<typeof TextPreviewDialog> = {
	title: "pages/AgentsPage/TextPreviewDialog",
	component: TextPreviewDialog,
};

export default meta;
type Story = StoryObj<typeof TextPreviewDialog>;

export const Default: Story = {
	args: {
		content:
			"This is some pasted text content.\nIt has multiple lines.\nAnd should be displayed in a readable format.",
		onClose: () => {},
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const dialog = await body.findByRole("dialog");
		expect(dialog).toBeInTheDocument();
		expect(
			within(dialog).getByText(/This is some pasted text content\./i),
		).toBeInTheDocument();
	},
};

export const LongContent: Story = {
	args: {
		content: Array(100)
			.fill(
				"This is a line of pasted text that demonstrates how the dialog handles very long content.",
			)
			.join("\n"),
		onClose: () => {},
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const dialog = await body.findByRole("dialog");
		const content = within(dialog).getByText(
			/This is a line of pasted text that demonstrates how the dialog handles very long content\./i,
		);
		expect(content).toBeInTheDocument();
		expect(content.parentElement).toHaveClass("overflow-auto");
	},
};

export const NoFileName: Story = {
	args: {
		content: "Some pasted content without a filename.",
		onClose: () => {},
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const dialog = await body.findByRole("dialog");
		expect(within(dialog).getByText("Pasted text")).toBeInTheDocument();
	},
};
