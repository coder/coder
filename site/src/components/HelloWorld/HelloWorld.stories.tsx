import type { Meta, StoryObj } from "@storybook/react-vite";
import { HelloWorld } from "./HelloWorld";

const meta: Meta<typeof HelloWorld> = {
	title: "components/HelloWorld",
	component: HelloWorld,
};

export default meta;
type Story = StoryObj<typeof HelloWorld>;

export const Default: Story = {};

export const CustomMessage: Story = {
	args: {
		children: "Hello from Coder!",
	},
};

export const WithCustomClassName: Story = {
	args: {
		className: "p-8 bg-surface-secondary rounded-lg",
		children: "Styled Hello World",
	},
};

export const LongMessage: Story = {
	args: {
		children:
			"This is a longer hello world message to test text wrapping and layout",
	},
};
