import type { Meta, StoryObj } from "@storybook/react";
import React from "react";
import { Slider } from "./Slider";

const meta: Meta<typeof Slider> = {
	title: "components/Slider",
	component: Slider,
	args: {},
	argTypes: {
		value: {
			control: "number",
			description: "The controlled value of the slider",
		},
		defaultValue: {
			control: "number",
			description: "The default value when initially rendered",
		},
		disabled: {
			control: "boolean",
			description:
				"When true, prevents the user from interacting with the slider",
		},
	},
};

export default meta;
type Story = StoryObj<typeof Slider>;

export const Default: Story = {};

export const Controlled: Story = {
	render: (args) => {
		const [value, setValue] = React.useState(50);
		return (
			<Slider {...args} value={[value]} onValueChange={([v]) => setValue(v)} />
		);
	},
	args: { value: [50], min: 0, max: 100, step: 1 },
};

export const Uncontrolled: Story = {
	args: { defaultValue: [30], min: 0, max: 100, step: 1 },
};

export const Disabled: Story = {
	args: { defaultValue: [40], disabled: true },
};

export const MultipleThumbs: Story = {
	args: {
		defaultValue: [20, 80],
		min: 0,
		max: 100,
		step: 5,
		minStepsBetweenThumbs: 1,
	},
};
