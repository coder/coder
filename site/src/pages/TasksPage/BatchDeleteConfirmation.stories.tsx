import { chromatic } from "testHelpers/chromatic";
import { MockTask, MockWorkspace } from "testHelpers/entities";
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
				initial_prompt: "Add comprehensive tests for the API endpoints",
				owner_name: "bob",
				created_at: new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString(),
				updated_at: new Date(Date.now() - 1 * 60 * 60 * 1000).toISOString(),
			},
			{
				...MockTask,
				id: "task-3",
				name: "task-docs-789",
				initial_prompt: "Update documentation for the new features",
				workspace_id: null,
				created_at: new Date(
					Date.now() - 3 * 24 * 60 * 60 * 1000,
				).toISOString(),
				updated_at: new Date(
					Date.now() - 2 * 24 * 60 * 60 * 1000,
				).toISOString(),
			},
		],
		workspaces: [
			MockWorkspace,
			{
				...MockWorkspace,
				id: "workspace-2",
				name: "bob-workspace",
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof BatchDeleteConfirmation>;

const Stage1_Consequences: Story = {};

const Stage2_ReviewTasks: Story = {
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

const Stage3_ReviewResources: Story = {
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

export {
	Stage1_Consequences as Consequences,
	Stage2_ReviewTasks as ReviewTasks,
	Stage3_ReviewResources as ReviewResources,
};
