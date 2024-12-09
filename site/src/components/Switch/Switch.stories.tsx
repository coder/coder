import type { Meta, StoryObj } from "@storybook/react";
import { Switch } from "./Switch";

const meta: Meta<typeof Switch> = {
	title: "components/Switch",
	component: Switch,
};

export default meta;
type Story = StoryObj<typeof Switch>;

export const DefaultOn: Story = {
	args: {
		checked: true,
		disabled: false,
	},
};

export const DefaultOff: Story = {
	args: {
		checked: false,
		disabled: false,
	},
};

export const DisabledOn: Story = {
	args: {
		checked: true,
		disabled: true,
	},
};

export const DisabledOff: Story = {
	args: {
		checked: false,
		disabled: true,
	},
};
