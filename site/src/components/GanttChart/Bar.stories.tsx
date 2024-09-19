import type { Meta, StoryObj } from "@storybook/react";
import { Bar } from "./Bar";
import { Label } from "./Label";

const meta: Meta<typeof Bar> = {
	title: "components/GanttChart/Bar",
	component: Bar,
	args: {
		width: 136,
	},
};

export default meta;
type Story = StoryObj<typeof Bar>;

export const Default: Story = {};

export const AfterLabel: Story = {
	args: {
		afterLabel: <Label color="secondary">5s</Label>,
	},
};

export const GreenColor: Story = {
	args: {
		color: "green",
	},
};
