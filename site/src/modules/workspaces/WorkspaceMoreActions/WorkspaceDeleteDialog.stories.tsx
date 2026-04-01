import {
	MockFailedWorkspace,
	MockTaskWorkspace,
	MockWorkspace,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { daysAgo } from "utils/time";
import { WorkspaceDeleteDialog } from "./WorkspaceDeleteDialog";

const meta: Meta<typeof WorkspaceDeleteDialog> = {
	title: "modules/workspaces/WorkspaceDeleteDialog",
	component: WorkspaceDeleteDialog,
	args: {
		workspace: {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				created_at: daysAgo(2),
			},
		},
		canDeleteFailedWorkspace: false,
		isOpen: true,
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceDeleteDialog>;

export const Example: Story = {};

// Should look the same as `Example`
export const Unhealthy: Story = {
	args: {
		workspace: MockFailedWorkspace,
	},
};

// Should look the same as `Example`
export const AdminView: Story = {
	args: {
		canDeleteFailedWorkspace: true,
	},
};

// Should show the `--orphan` option
export const UnhealthyAdminView: Story = {
	args: {
		workspace: MockFailedWorkspace,
		canDeleteFailedWorkspace: true,
	},
};

export const WithTask: Story = {
	args: {
		workspace: MockTaskWorkspace,
	},
};
