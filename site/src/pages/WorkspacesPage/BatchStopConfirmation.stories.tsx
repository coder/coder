import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { MockWorkspace, mockApiError } from "#/testHelpers/entities";
import { BatchStopConfirmation } from "./BatchStopConfirmation";

const meta: Meta<typeof BatchStopConfirmation> = {
	title: "pages/WorkspacesPage/BatchStopConfirmation",
	component: BatchStopConfirmation,
	args: {
		onClose: action("onClose"),
		onConfirm: action("onConfirm"),
		open: true,
		workspacesToStop: [
			MockWorkspace,
			{
				...MockWorkspace,
				id: "workspace-2",
				name: "Test-Workspace-2",
			},
			{
				...MockWorkspace,
				id: "workspace-3",
				name: "Test-Workspace-3",
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof BatchStopConfirmation>;

export const Example: Story = {};

export const FailedStop: Story = {
	args: {
		error: mockApiError({
			message: "Failed to stop workspaces.",
			detail: "Test-Workspace-2 is already busy with a build.",
		}),
	},
};
