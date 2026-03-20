import type { Meta, StoryObj } from "@storybook/react-vite";
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
};

export const NoFileName: Story = {
	args: {
		content: "Some pasted content without a filename.",
		onClose: () => {},
	},
};
