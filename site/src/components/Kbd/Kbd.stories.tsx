import type { Meta, StoryObj } from "@storybook/react-vite";
import { CommandIcon } from "lucide-react";
import { Kbd, KbdGroup } from "./Kbd";

const meta: Meta<typeof Kbd> = {
	title: "components/Kbd",
	component: Kbd,
	args: {
		children: "K",
	},
};

export default meta;
type Story = StoryObj<typeof Kbd>;

export const Default: Story = {};

export const WithIcon: Story = {
	args: {
		children: <CommandIcon />,
	},
};

export const Group: Story = {
	render: () => (
		<KbdGroup>
			<Kbd>
				<CommandIcon />
			</Kbd>
			<Kbd>Enter</Kbd>
		</KbdGroup>
	),
};

export const MultipleKeys: Story = {
	render: () => (
		<KbdGroup>
			<Kbd>Ctrl</Kbd>
			<Kbd>Shift</Kbd>
			<Kbd>P</Kbd>
		</KbdGroup>
	),
};
