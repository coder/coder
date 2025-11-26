import { chromatic } from "testHelpers/chromatic";
import { MockTask } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { userEvent, within } from "storybook/test";
import { BatchDeleteConfirmation } from "./BatchDeleteConfirmation";

const meta: Meta<typeof BatchDeleteConfirmation> = {
	title: "pages/TasksPage/BatchDeleteConfirmation",
	parameters: { chromatic },
	component: BatchDeleteConfirmation,
	args: {
		onClose: action("onClose"),
		onConfirm: action("onConfirm"),
		open: true,
		isLoading: false,
		checkedTasks: [
			MockTask,
			{
				...MockTask,
				id: "task-2",
				name: "task-test-456",
				display_name: "Add API Tests",
				initial_prompt: "Add comprehensive tests for the API endpoints",
				// Different owner to test admin bulk delete of other users' tasks
				owner_name: "bob",
				created_at: new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString(),
				updated_at: new Date(Date.now() - 1 * 60 * 60 * 1000).toISOString(),
			},
			{
				...MockTask,
				id: "task-3",
				name: "task-docs-789",
				display_name: "Update Documentation",
				initial_prompt: "Update documentation for the new features",
				// Intentionally null to test that only 2 workspaces are shown in review resources stage
				workspace_id: null,
				created_at: new Date(
					Date.now() - 3 * 24 * 60 * 60 * 1000,
				).toISOString(),
				updated_at: new Date(
					Date.now() - 2 * 24 * 60 * 60 * 1000,
				).toISOString(),
			},
		],
		workspaceCount: 2,
	},
};

export default meta;
type Story = StoryObj<typeof BatchDeleteConfirmation>;

export const Consequences: Story = {};

export const ReviewTasks: Story = {
	play: async ({ canvasElement, step }) => {
		const body = within(canvasElement.ownerDocument.body);

		await step("Advance to stage 2: Review tasks", async () => {
			const confirmButton = await body.findByRole("button", {
				name: /review selected tasks/i,
			});
			await userEvent.click(confirmButton);
		});
	},
};

export const ReviewResources: Story = {
	play: async ({ canvasElement, step }) => {
		const body = within(canvasElement.ownerDocument.body);

		await step("Advance to stage 2: Review tasks", async () => {
			const confirmButton = await body.findByRole("button", {
				name: /review selected tasks/i,
			});
			await userEvent.click(confirmButton);
		});

		await step("Advance to stage 3: Review resources", async () => {
			const confirmButton = await body.findByRole("button", {
				name: /confirm.*tasks/i,
			});
			await userEvent.click(confirmButton);
		});
	},
};
