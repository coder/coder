import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import { chromatic } from "testHelpers/chromatic";
import { MockUserMember, MockWorkspace } from "testHelpers/entities";
import { BatchDeleteConfirmation } from "./BatchDeleteConfirmation";

const meta: Meta<typeof BatchDeleteConfirmation> = {
	title: "pages/WorkspacesPage/BatchDeleteConfirmation",
	parameters: { chromatic },
	component: BatchDeleteConfirmation,
	args: {
		onClose: action("onClose"),
		onConfirm: action("onConfirm"),
		open: true,
		checkedWorkspaces: [
			MockWorkspace,
			{
				...MockWorkspace,
				name: "Test-Workspace-2",
				last_used_at: "2023-08-16T15:29:10.302441433Z",
				owner_id: MockUserMember.id,
				owner_name: MockUserMember.username,
			},
			{
				...MockWorkspace,
				name: "Test-Workspace-3",
				last_used_at: "2023-11-16T15:29:10.302441433Z",
				owner_id: MockUserMember.id,
				owner_name: MockUserMember.username,
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof BatchDeleteConfirmation>;

const Example: Story = {};

export { Example as BatchDeleteConfirmation };
