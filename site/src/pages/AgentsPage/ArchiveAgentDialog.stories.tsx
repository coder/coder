import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { ArchiveAgentDialog } from "./ArchiveAgentDialog";

const meta: Meta<typeof ArchiveAgentDialog> = {
	title: "pages/AgentsPage/ArchiveAgentDialog",
	component: ArchiveAgentDialog,
	args: {
		open: true,
		onClose: fn(),
		onArchiveOnly: fn(),
		onArchiveAndDeleteWorkspace: fn(),
		chatTitle: "Fix authentication bug",
		isLoading: false,
	},
};

export default meta;
type Story = StoryObj<typeof ArchiveAgentDialog>;

export const Default: Story = {};

export const Loading: Story = {
	args: {
		isLoading: true,
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const archiveOnlyButton = await body.findByRole("button", {
			name: /Archive only/i,
		});
		expect(archiveOnlyButton).toBeDisabled();
		const deleteButton = await body.findByRole("button", {
			name: /Archive.*Delete Workspace/i,
		});
		expect(deleteButton).toBeDisabled();
	},
};

export const WithCheckboxChecked: Story = {
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const deleteButton = await body.findByRole("button", {
			name: /Archive.*Delete Workspace/i,
		});
		expect(deleteButton).toBeDisabled();

		const checkbox = await body.findByRole("checkbox", {
			name: /Also delete the associated workspace/i,
		});
		await userEvent.click(checkbox);

		expect(deleteButton).toBeEnabled();
	},
};

export const ArchiveOnlyClick: Story = {
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		const archiveOnlyButton = await body.findByRole("button", {
			name: /Archive only/i,
		});
		await userEvent.click(archiveOnlyButton);
		expect(args.onArchiveOnly).toHaveBeenCalledTimes(1);
	},
};

export const CheckboxThenDeleteClick: Story = {
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		const checkbox = await body.findByRole("checkbox", {
			name: /Also delete the associated workspace/i,
		});
		await userEvent.click(checkbox);

		const deleteButton = await body.findByRole("button", {
			name: /Archive.*Delete Workspace/i,
		});
		expect(deleteButton).toBeEnabled();
		await userEvent.click(deleteButton);
		expect(args.onArchiveAndDeleteWorkspace).toHaveBeenCalledTimes(1);
	},
};
