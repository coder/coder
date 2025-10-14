import { MockTask, mockApiError } from "testHelpers/entities";
import { withGlobalSnackbar } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import { expect, spyOn, userEvent, within } from "storybook/test";
import { TaskFeedbackDialog } from "./TaskFeedbackDialog";

const meta: Meta<typeof TaskFeedbackDialog> = {
	title: "modules/tasks/TaskFeedbackDialog",
	component: TaskFeedbackDialog,
	args: {
		taskId: MockTask.id,
		open: true,
	},
};

export default meta;
type Story = StoryObj<typeof TaskFeedbackDialog>;

export const Idle: Story = {};

export const Submitting: Story = {
	beforeEach: async () => {
		spyOn(API.experimental, "createTaskFeedback").mockImplementation(() => {
			return new Promise(() => {});
		});
	},
	play: async ({ canvasElement, step }) => {
		const body = within(canvasElement.ownerDocument.body);

		step("fill and submit the form", async () => {
			const regularOption = body.getByLabelText(
				"It sort of worked, but struggled a lot",
			);
			userEvent.click(regularOption);

			const commentTextarea = body.getByRole("textbox", {
				name: "Additional comments",
			});
			await userEvent.type(commentTextarea, "This is my comment");

			const submitButton = body.getByRole("button", {
				name: "Submit Feedback",
			});
			await userEvent.click(submitButton);
		});
	},
};

export const Success: Story = {
	args: {
		open: true,
	},
	decorators: [withGlobalSnackbar],
	beforeEach: async () => {
		spyOn(API.experimental, "createTaskFeedback").mockResolvedValue();
	},
	play: async ({ canvasElement, step }) => {
		const body = within(canvasElement.ownerDocument.body);

		step("fill and submit the form", async () => {
			const regularOption = body.getByLabelText(
				"It sort of worked, but struggled a lot",
			);
			userEvent.click(regularOption);

			const commentTextarea = body.getByRole("textbox", {
				name: "Additional comments",
			});
			await userEvent.type(commentTextarea, "This is my comment");

			const submitButton = body.getByRole("button", {
				name: "Submit Feedback",
			});
			await userEvent.click(submitButton);
		});

		step("submitted successfully", async () => {
			await body.findByText("Feedback submitted successfully");
			expect(API.experimental.createTaskFeedback).toHaveBeenCalledWith(
				MockTask.id,
				{
					rate: "regular",
					comment: "This is my comment",
				},
			);
		});
	},
};

export const Failure: Story = {
	beforeEach: async () => {
		spyOn(API.experimental, "createTaskFeedback").mockRejectedValue(
			mockApiError({
				message: "Failed to submit feedback",
				detail: "Server is down",
			}),
		);
	},
	play: async ({ canvasElement, step }) => {
		const body = within(canvasElement.ownerDocument.body);

		step("fill and submit the form", async () => {
			const regularOption = body.getByLabelText(
				"It sort of worked, but struggled a lot",
			);
			userEvent.click(regularOption);

			const commentTextarea = body.getByRole("textbox", {
				name: "Additional comments",
			});
			await userEvent.type(commentTextarea, "This is my comment");

			const submitButton = body.getByRole("button", {
				name: "Submit Feedback",
			});
			await userEvent.click(submitButton);
		});
	},
};
