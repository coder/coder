import type { Meta, StoryObj } from "@storybook/react-vite";
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

export const SmallOn: Story = {
	args: {
		checked: true,
		disabled: false,
		size: "sm",
	},
};

export const SmallOff: Story = {
	args: {
		checked: false,
		disabled: false,
		size: "sm",
	},
};

export const SmallDisabledOn: Story = {
	args: {
		checked: true,
		disabled: true,
		size: "sm",
	},
};

export const SmallDisabledOff: Story = {
	args: {
		checked: false,
		disabled: true,
		size: "sm",
	},
};
