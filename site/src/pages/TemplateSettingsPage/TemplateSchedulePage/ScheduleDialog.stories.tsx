import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { ScheduleDialog } from "./ScheduleDialog";

const meta: Meta<typeof ScheduleDialog> = {
	title: "pages/TemplateSettingsPage/ScheduleDialog",
	component: ScheduleDialog,
	args: {
		onConfirm: action("onConfirm"),
		onClose: action("onClose"),
		updateDormantWorkspaces: action("updateDormantWorkspaces"),
		updateInactiveWorkspaces: action("updateInactiveWorkspaces"),
		open: true,
		title: "Workspace Scheduling",
		inactiveWorkspacesToGoDormant: 0,
		inactiveWorkspacesToGoDormantInWeek: 0,
		dormantWorkspacesToBeDeleted: 0,
		dormantWorkspacesToBeDeletedInWeek: 0,
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
