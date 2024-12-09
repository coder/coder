import type { Meta, StoryObj } from "@storybook/react";
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
