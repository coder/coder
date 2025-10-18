import type { Meta, StoryObj } from "@storybook/react-vite";
import { Input } from "./Input";

const meta: Meta<typeof Input> = {
	title: "components/Input",
	component: Input,
	args: {
		placeholder: "Placeholder",
	},
};

export default meta;
type Story = StoryObj<typeof Input>;

export const Default: Story = {};

export const Disabled: Story = {
	args: {
		disabled: true,
	},
};

export const TimeInput: Story = {
	args: {
		type: "time",
		defaultValue: "12:30",
	},
};

export const TimeInputAfterClick: Story = {
	name: "TimeInput (after click)",
	args: {
		type: "time",
		defaultValue: "12:30",
	},

	play: async ({ canvasElement }) => {
		const input = canvasElement.querySelector(
			'input[type="time"]',
		) as HTMLInputElement | null;
		input?.click();
	},
};
