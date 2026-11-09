import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { UnsavedChangesDialog } from "./UnsavedChangesDialog";

const meta: Meta<typeof UnsavedChangesDialog> = {
	title: "components/Dialog/UnsavedChangesDialog",
	component: UnsavedChangesDialog,
	args: {
		open: true,
		onClose: fn(),
		onConfirm: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof UnsavedChangesDialog>;

export const Default: Story = {};
