import type { Meta, StoryObj } from "@storybook/react";
import { Trash } from "lucide-react";
import { Button } from "./Button";

const meta: Meta<typeof Button> = {
	title: "components/Button",
	component: Button,
	args: {
		children: (
			<>
				<Trash />
				Button
			</>
		),
	},
};

export default meta;
type Story = StoryObj<typeof Button>;

export const Default: Story = {};

export const DefaultDisabled: Story = {
	args: {
		disabled: true,
	},
};

export const Outline: Story = {
	args: {
		variant: "outline",
	},
};

export const OutlineDisabled: Story = {
	args: {
		variant: "outline",
		disabled: true,
	},
};

export const Subtle: Story = {
	args: {
		variant: "subtle",
	},
};

export const SubtleDisabled: Story = {
	args: {
		variant: "subtle",
		disabled: true,
	},
};

export const Destructive: Story = {
	args: {
		variant: "destructive",
		children: "Delete",
	},
};

export const DestructiveDisabled: Story = {
	args: {
		...Destructive.args,
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
