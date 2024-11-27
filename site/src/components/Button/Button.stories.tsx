import type { Meta, StoryObj } from "@storybook/react";
import { Trash } from "lucide-react";
import { Button } from "./Button";

const meta: Meta<typeof Button> = {
	title: "components/Button",
	component: Button,
	args: {
		children: "Button",
	},
};

export default meta;
type Story = StoryObj<typeof Button>;

export const Default: Story = {args: {
	children: <><Trash />Button</>,
}};

export const Outline: Story = {args: {
	variant: "outline",
	children: <><Trash />Button</>,
}};

export const Subtle: Story = {
	args: {
		variant: "subtle",
		children: <><Trash />Button</>,
	},
};

export const Warning: Story = {
	args: {
		variant: "warning",
		children: <><Trash />Button</>,
	},
};

export const DefaultDisabled: Story = {
	args: {
		disabled: true,
		children: <><Trash />Button</>,
	},
};

export const OutlineDisabled: Story = {
	args: {
		variant: "outline",
		children: <><Trash />Button</>,
		disabled: true,
	},
};

export const SubtleDisabled: Story = {
	args: {
		variant: "subtle",
		children: <><Trash />Button</>,
		disabled: true,
	},
};

export const IconButtonDefault: Story = {
	args: {
		variant: "default",
		children: <Trash />,
	},
};

export const IconButtonOutline: Story = {
	args: {
		variant: "outline",
		children: <Trash />,
	},
};

export const IconButtonSubtle: Story = {
	args: {
		variant: "subtle",
		children: <Trash />,
	},
};

