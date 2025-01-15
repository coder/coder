import type { Meta, StoryObj } from "@storybook/react";
import { Link } from "./Link";

const meta: Meta<typeof Link> = {
	title: "components/Link",
	component: Link,
	args: {
		text: "Learn more",
	},
};

export default meta;
type Story = StoryObj<typeof Link>;

export const Default: Story = {};

export const Small: Story = {
	args: {
		size: "sm",
	},
};
