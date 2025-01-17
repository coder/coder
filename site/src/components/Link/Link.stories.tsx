import type { Meta, StoryObj } from "@storybook/react";
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
