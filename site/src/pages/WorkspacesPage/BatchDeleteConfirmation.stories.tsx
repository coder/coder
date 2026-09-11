import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { userEvent, within } from "storybook/test";
import {
	MockUserMember,
	MockWorkspace,
	mockApiError,
} from "#/testHelpers/entities";
import { BatchDeleteConfirmation } from "./BatchDeleteConfirmation";

const meta: Meta<typeof BatchDeleteConfirmation> = {
	title: "pages/WorkspacesPage/BatchDeleteConfirmation",
	component: BatchDeleteConfirmation,
	args: {
		onClose: action("onClose"),
		onConfirm: action("onConfirm"),
		open: true,
		checkedWorkspaces: [
			MockWorkspace,
			{
				...MockWorkspace,
				id: "workspace-2",
				name: "Test-Workspace-2",
				last_used_at: "2023-08-16T15:29:10.302441433Z",
				owner_id: MockUserMember.id,
				owner_name: MockUserMember.username,
			},
			{
				...MockWorkspace,
				id: "workspace-3",
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

export const FailedDelete: Story = {
	args: {
		error: mockApiError({
			message: "Failed to delete some workspaces.",
			detail: "Test-Workspace-2 is already busy with a build.",
		}),
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);
		await user.click(
			body.getByRole("button", { name: /review selected workspaces/i }),
		);
		await user.click(
			body.getByRole("button", { name: /confirm 3 workspaces/i }),
		);
	},
};

export { Example as BatchDeleteConfirmation };
