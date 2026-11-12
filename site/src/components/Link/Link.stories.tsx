import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent } from "storybook/test";
import { Link } from "./Link";

const meta: Meta<typeof Link> = {
	title: "components/Link",
	component: Link,
	args: {
		children: "Learn more",
	},
};

export default meta;
type Story = StoryObj<typeof Link>;

export const Large: Story = {};

export const Small: Story = {
	args: {
		size: "sm",
	},
};

// The focus ring outlines the full clickable box, which must be at least
// 24px tall (WCAG 2.5.8).
export const SmallFocused: Story = {
	args: {
		size: "sm",
		href: "#",
	},
	play: async () => {
		await userEvent.tab();
	},
};

export const InlineUsage: Story = {
	render: () => {
		return (
			<p className="text-sm">
				A <Link>workspace</Link> is your personal, customized development
				environment. It's based on a <Link>template</Link> that configures your
				workspace using Terraform.
			</p>
		);
	},
};
