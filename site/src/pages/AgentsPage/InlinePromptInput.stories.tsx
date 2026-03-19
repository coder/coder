import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { InlinePromptInput } from "./RemoteDiffPanel";

const meta: Meta<typeof InlinePromptInput> = {
	title: "pages/AgentsPage/InlinePromptInput",
	component: InlinePromptInput,
	decorators: [
		(Story) => (
			<div className="w-[500px] rounded-lg bg-surface-primary p-4">
				<Story />
			</div>
		),
	],
	args: {
		onSubmit: fn(),
		onCancel: fn(),
	},
};
export default meta;
type Story = StoryObj<typeof InlinePromptInput>;

export const Default: Story = {};

export const WithText: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const textarea = canvas.getByPlaceholderText("Add a comment...");
		await userEvent.type(textarea, "Fix the race condition on line 42");
	},
};

export const Submitting: Story = {
	args: {
		onSubmit: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const textarea = canvas.getByPlaceholderText("Add a comment...");
		await userEvent.type(textarea, "Fix the race condition on line 42");
		await userEvent.keyboard("{Enter}");
		await expect(args.onSubmit).toHaveBeenCalledWith(
			"Fix the race condition on line 42",
		);
	},
};

export const Cancelling: Story = {
	args: {
		onCancel: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const textarea = canvas.getByPlaceholderText("Add a comment...");
		await userEvent.click(textarea);
		await userEvent.keyboard("{Escape}");
		await expect(args.onCancel).toHaveBeenCalled();
	},
};
