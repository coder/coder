import type { Meta, StoryObj } from "@storybook/react";
import { PlusIcon } from "lucide-react";
import { Button } from "./Button";

const meta: Meta<typeof Button> = {
	title: "components/Button",
	component: Button,
	args: {
		children: (
			<>
				<PlusIcon />
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

export const DefaultSmall: Story = {
	args: {
		size: "sm",
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

export const OutlineSmall: Story = {
	args: {
		variant: "outline",
		size: "sm",
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

export const SubtleSmall: Story = {
	args: {
		variant: "subtle",
		size: "sm",
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

export const DestructiveSmall: Story = {
	args: {
		...Destructive.args,
		size: "sm",
	},
};

export const IconButtonDefault: Story = {
	args: {
		variant: "default",
		size: "icon",
		children: <PlusIcon />,
	},
};

export const IconButtonOutline: Story = {
	args: {
		variant: "outline",
		size: "icon",
		children: <PlusIcon />,
	},
};

export const IconButtonSubtle: Story = {
	args: {
		variant: "subtle",
		size: "icon",
		children: <PlusIcon />,
	},
};
