import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { ScheduleDialog } from "./ScheduleDialog";

const meta: Meta<typeof ScheduleDialog> = {
	title: "pages/TemplatePage/TemplateSettingsPage/ScheduleDialog",
	component: ScheduleDialog,
	args: {
		onConfirm: fn(),
		onClose: fn(),
		updateDormantWorkspaces: fn(),
		updateInactiveWorkspaces: fn(),
		open: true,
		title: "Workspace Scheduling",
		inactiveWorkspacesToGoDormant: 0,
		inactiveWorkspacesToGoDormantInWeek: 0,
		dormantWorkspacesToBeDeleted: 0,
		dormantWorkspacesToBeDeletedInWeek: 0,
		dormantWorkspacesChecked: false,
		inactiveWorkspacesChecked: false,
		dormantValueChanged: false,
		deletionValueChanged: false,
	},
};

export default meta;
type Story = StoryObj<typeof ScheduleDialog>;

export const DormancyThreshold: Story = {
	args: {
		dormantValueChanged: true,
		inactiveWorkspacesToGoDormant: 1,
		inactiveWorkspacesToGoDormantInWeek: 5,
	},
};

export const DormancyDeletion: Story = {
	args: {
		deletionValueChanged: true,
		dormantWorkspacesToBeDeleted: 1,
		dormantWorkspacesToBeDeletedInWeek: 5,
	},
};

export const PreventDormancyAndSubmit: Story = {
	args: {
		dormantValueChanged: true,
		inactiveWorkspacesToGoDormant: 1,
		inactiveWorkspacesToGoDormantInWeek: 5,
	},
	play: async ({ args, step }) => {
		const body = within(document.body);

		await step("selecting prevention resets inactivity periods", async () => {
			await userEvent.click(await body.findByRole("checkbox"));
			await expect(args.updateInactiveWorkspaces).toHaveBeenCalledWith(true);
		});

		await step("submitting confirms the schedule change", async () => {
			await userEvent.click(body.getByRole("button", { name: "Submit" }));
			await expect(args.onConfirm).toHaveBeenCalled();
		});
	},
};

export const PreventDeletionAndSubmit: Story = {
	args: {
		deletionValueChanged: true,
		dormantWorkspacesToBeDeleted: 1,
		dormantWorkspacesToBeDeletedInWeek: 5,
	},
	play: async ({ args, step }) => {
		const body = within(document.body);

		await step("selecting prevention resets dormancy periods", async () => {
			await userEvent.click(await body.findByRole("checkbox"));
			await expect(args.updateDormantWorkspaces).toHaveBeenCalledWith(true);
		});

		await step("submitting confirms the schedule change", async () => {
			await userEvent.click(body.getByRole("button", { name: "Submit" }));
			await expect(args.onConfirm).toHaveBeenCalled();
		});
	},
};

export const CancelClosesDialog: Story = {
	args: {
		dormantValueChanged: true,
		inactiveWorkspacesToGoDormant: 1,
		inactiveWorkspacesToGoDormantInWeek: 5,
	},
	play: async ({ args }) => {
		const body = within(document.body);
		await userEvent.click(await body.findByRole("button", { name: "Cancel" }));
		await expect(args.onClose).toHaveBeenCalled();
	},
};
