import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import { ScheduleDialog } from "./ScheduleDialog";

const meta: Meta<typeof ScheduleDialog> = {
	title: "pages/TemplateSettingsPage/ScheduleDialog",
	component: ScheduleDialog,
	args: {
		onConfirm: action("onConfirm"),
		onClose: action("onClose"),
		open: true,
		title: "Workspace Scheduling",
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
