import type { Meta, StoryObj } from "@storybook/react-vite";
import { Link as RouterLink } from "react-router";
import { userEvent } from "storybook/test";
import { Link } from "./Link";

const meta: Meta<typeof Link> = {
	title: "components/Link",
	component: Link,
	args: {
		children: "Learn more",
		href: "https://docs.example.com",
		target: "_blank",
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

export const Internal: Story = {
	args: {
		children: "Go to workspaces",
		href: "/workspaces",
		target: undefined,
	},
};

export const InternalRouterLink: Story = {
	render: () => (
		<Link asChild>
			<RouterLink to="/workspaces">Go to workspaces</RouterLink>
		</Link>
	),
};

export const ExternalSameTab: Story = {
	args: {
		target: undefined,
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
