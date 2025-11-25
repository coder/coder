import { MockTask } from "testHelpers/entities";
import { withGlobalSnackbar } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { TaskDeleteDialog } from "./TaskDeleteDialog";

const meta: Meta<typeof TaskDeleteDialog> = {
	title: "modules/tasks/TaskDeleteDialog",
	component: TaskDeleteDialog,
	decorators: [withGlobalSnackbar],
};

export default meta;
type Story = StoryObj<typeof TaskDeleteDialog>;

export const DeleteTaskSuccess: Story = {
	decorators: [withGlobalSnackbar],
	args: {
		open: true,
		task: MockTask,
		onClose: () => {},
	},
	parameters: {
		chromatic: {
			disableSnapshot: false,
		},
	},
	beforeEach: () => {
		spyOn(API.tasks, "deleteTask").mockResolvedValue();
	},
	play: async ({ canvasElement, step }) => {
		const body = within(canvasElement.ownerDocument.body);

		await step("Confirm delete", async () => {
			const confirmButton = await body.findByRole("button", {
				name: /delete/i,
			});
			await userEvent.click(confirmButton);
			await step("Confirm delete", async () => {
				await waitFor(() => {
					expect(API.tasks.deleteTask).toHaveBeenCalledWith(
						MockTask.owner_name,
						MockTask.id,
					);
				});
			});
		});
	},
};
