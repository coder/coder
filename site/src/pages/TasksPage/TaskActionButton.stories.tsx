import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { TaskActionButton } from "./TaskActionButton";

const meta: Meta<typeof TaskActionButton> = {
	title: "pages/TasksPage/TaskActionButton",
	component: TaskActionButton,
	args: {
		onClick: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof TaskActionButton>;

export const Pause: Story = {
	args: {
		action: "pause",
	},
};

export const Resume: Story = {
	args: {
		action: "resume",
	},
};

export const Loading: Story = {
	args: {
		action: "pause",
		loading: true,
	},
};

export const Disabled: Story = {
	args: {
		action: "pause",
		disabled: true,
	},
};

export const ClickHandler: Story = {
	args: {
		action: "pause",
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button", { name: /pause task/i });
		await userEvent.click(button);
		expect(args.onClick).toHaveBeenCalledTimes(1);
	},
};
